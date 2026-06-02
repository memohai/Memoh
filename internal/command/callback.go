package command

import (
	"fmt"
	"hash/fnv"
	"net/url"
	"strconv"
	"strings"
	"sync"
)

// callbackNamespace prefixes every interactive callback_data string so it does
// not collide with the existing "approve:"/"reject:" tool-approval callbacks.
const callbackNamespace = "m~"

// telegramCallbackLimit is Telegram's hard limit on callback_data (64 bytes).
const telegramCallbackLimit = 64

// Callback kinds returned by DecodeCallback.
const (
	callbackKindListPage      = "list_page"
	callbackKindModelProvider = "model_provider"
	callbackKindModelSelect   = "model_select"
	callbackKindRange         = "range"
	callbackKindConfirmNew    = "confirm_new"
	callbackKindDismiss       = "dismiss"
	callbackKindNoop          = "noop"
)

// ParsedCallback is the decoded form of an interactive callback_data string.
type ParsedCallback struct {
	Kind          string
	Resource      string
	Action        string
	Args          []string
	Page          int
	ProviderIndex int
	FlatIndex     int
	Range         string
}

// IsInteractiveCallback reports whether data is one of our interactive
// callbacks (as opposed to a tool-approval callback or unrelated data).
func IsInteractiveCallback(data string) bool {
	return strings.HasPrefix(data, callbackNamespace)
}

// IsDismiss reports whether the callback closes the interactive message.
func (p ParsedCallback) IsDismiss() bool { return p.Kind == callbackKindDismiss }

// IsNoop reports whether the callback is inert (e.g. the page indicator).
func (p ParsedCallback) IsNoop() bool { return p.Kind == callbackKindNoop }

// DismissCallback returns the callback_data that closes an interactive message.
func DismissCallback() string { return callbackNamespace + "x" }

// NoopCallback returns the callback_data for inert buttons (e.g. the page
// indicator) that should be acknowledged but otherwise ignored.
func NoopCallback() string { return callbackNamespace + "noop" }

// EncodeListCallback builds the callback_data for a list-pagination button.
// Layout: "m~lp~{resource}~{action}~{page}~{argsToken}". When the encoded args
// would push the string past Telegram's 64-byte limit, the args are stashed in
// a bounded process-local table and referenced by a short token ("#<hash>").
func EncodeListCallback(resource, action string, args []string, page int) string {
	base := fmt.Sprintf("%slp~%s~%s~%d~", callbackNamespace, resource, action, page)
	argsStr := strings.TrimSpace(strings.Join(args, " "))
	if argsStr == "" {
		return base
	}
	encoded := base + url.QueryEscape(argsStr)
	if len(encoded) <= telegramCallbackLimit {
		return encoded
	}
	return base + "#" + stashArgs(argsStr)
}

// EncodeModelProviderCallback builds the callback_data for drilling into a
// provider's paginated model list. Layout: "m~mpl~{providerIndex}~{page}".
func EncodeModelProviderCallback(providerIndex, page int) string {
	return fmt.Sprintf("%smpl~%d~%d", callbackNamespace, providerIndex, page)
}

// EncodeModelSelectCallback builds the callback_data for selecting a model by
// its global flat index. Layout: "m~ms~{flatIndex}". The index avoids embedding
// long model IDs that would breach the 64-byte limit.
func EncodeModelSelectCallback(flatIndex int) string {
	return fmt.Sprintf("%sms~%d", callbackNamespace, flatIndex)
}

// EncodeRangeCallback builds the callback_data for a time-window preset button.
// Layout: "m~rg~{resource}~{action}~{rangeKey}".
func EncodeRangeCallback(resource, action, rangeKey string) string {
	return fmt.Sprintf("%srg~%s~%s~%s", callbackNamespace, resource, action, rangeKey)
}

// EncodeConfirmNewCallback builds the callback_data for confirming a /new reset.
// Layout: "m~cn~{mode}" where mode is chat|discuss. Tapping re-dispatches
// "/new {mode} --confirm", which performs the actual session reset.
func EncodeConfirmNewCallback(mode string) string {
	return fmt.Sprintf("%scn~%s", callbackNamespace, mode)
}

// DecodeCallback parses an interactive callback_data string. The bool is false
// for data that is not one of our interactive callbacks.
func DecodeCallback(data string) (ParsedCallback, bool) {
	if !strings.HasPrefix(data, callbackNamespace) {
		return ParsedCallback{}, false
	}
	body := strings.TrimPrefix(data, callbackNamespace)
	switch {
	case body == "x":
		return ParsedCallback{Kind: callbackKindDismiss}, true
	case body == "noop":
		return ParsedCallback{Kind: callbackKindNoop}, true
	case strings.HasPrefix(body, "lp~"):
		parts := strings.SplitN(strings.TrimPrefix(body, "lp~"), "~", 4)
		if len(parts) < 3 {
			return ParsedCallback{}, false
		}
		page, err := strconv.Atoi(parts[2])
		if err != nil || page < 0 {
			return ParsedCallback{}, false
		}
		var args []string
		if len(parts) == 4 {
			args = decodeArgsToken(parts[3])
		}
		return ParsedCallback{
			Kind:     callbackKindListPage,
			Resource: parts[0],
			Action:   parts[1],
			Page:     page,
			Args:     args,
		}, true
	case strings.HasPrefix(body, "mpl~"):
		parts := strings.SplitN(strings.TrimPrefix(body, "mpl~"), "~", 2)
		if len(parts) != 2 {
			return ParsedCallback{}, false
		}
		prov, errP := strconv.Atoi(parts[0])
		page, errPg := strconv.Atoi(parts[1])
		if errP != nil || errPg != nil || prov < 0 || page < 0 {
			return ParsedCallback{}, false
		}
		return ParsedCallback{Kind: callbackKindModelProvider, ProviderIndex: prov, Page: page}, true
	case strings.HasPrefix(body, "ms~"):
		flat, err := strconv.Atoi(strings.TrimPrefix(body, "ms~"))
		if err != nil || flat < 0 {
			return ParsedCallback{}, false
		}
		return ParsedCallback{Kind: callbackKindModelSelect, FlatIndex: flat}, true
	case strings.HasPrefix(body, "rg~"):
		parts := strings.SplitN(strings.TrimPrefix(body, "rg~"), "~", 3)
		if len(parts) != 3 || parts[2] == "" {
			return ParsedCallback{}, false
		}
		return ParsedCallback{Kind: callbackKindRange, Resource: parts[0], Action: parts[1], Range: parts[2]}, true
	case strings.HasPrefix(body, "cn~"):
		mode := strings.TrimPrefix(body, "cn~")
		if mode == "" {
			return ParsedCallback{}, false
		}
		return ParsedCallback{Kind: callbackKindConfirmNew, Action: mode}, true
	}
	return ParsedCallback{}, false
}

// SyntheticCommand returns the slash command text to re-dispatch for a parsed
// callback, or "" when the callback has no command (dismiss/noop).
func (p ParsedCallback) SyntheticCommand() string {
	switch p.Kind {
	case callbackKindListPage:
		var b strings.Builder
		b.WriteString("/")
		b.WriteString(p.Resource)
		b.WriteString(" ")
		b.WriteString(p.Action)
		if len(p.Args) > 0 {
			b.WriteString(" ")
			b.WriteString(strings.Join(p.Args, " "))
		}
		b.WriteString(" --page ")
		b.WriteString(strconv.Itoa(p.Page))
		return b.String()
	case callbackKindModelProvider:
		return fmt.Sprintf("/model list --prov %d --page %d", p.ProviderIndex, p.Page)
	case callbackKindModelSelect:
		return fmt.Sprintf("/model set --flat %d", p.FlatIndex)
	case callbackKindRange:
		return fmt.Sprintf("/%s %s --range %s", p.Resource, p.Action, p.Range)
	case callbackKindConfirmNew:
		return fmt.Sprintf("/new %s --confirm", p.Action)
	default:
		return ""
	}
}

// decodeArgsToken decodes the args segment of a callback, resolving the stashed
// token form ("#<hash>") when present. A miss returns nil (unfiltered).
//
// A miss can happen when an old paginated keyboard is tapped after the
// bounded stash (256 entries, FIFO) has rolled past the original entry.
// The downstream synthetic command then re-runs the list without the
// narrowing args, showing the user an unfiltered view rather than the
// filtered subset they originally requested.
func decodeArgsToken(token string) []string {
	if token == "" {
		return nil
	}
	if strings.HasPrefix(token, "#") {
		hash := strings.TrimPrefix(token, "#")
		argsStashMu.Lock()
		stored := argsStash[hash]
		argsStashMu.Unlock()
		if stored == "" {
			return nil
		}
		return strings.Fields(stored)
	}
	decoded, err := url.QueryUnescape(token)
	if err != nil {
		return nil
	}
	decoded = strings.TrimSpace(decoded)
	if decoded == "" {
		return nil
	}
	return strings.Fields(decoded)
}

// Bounded process-local table for callback args too long to inline. This is
// ephemeral presentation state (a keyboard's lifetime), not persisted user
// preference, so it carries no backend/semantic meaning.
var (
	argsStashMu    sync.Mutex
	argsStash      = make(map[string]string)
	argsStashOrder []string
)

const argsStashLimit = 256

func stashArgs(args string) string {
	token := shortHash(args)
	argsStashMu.Lock()
	defer argsStashMu.Unlock()
	if _, ok := argsStash[token]; !ok {
		argsStash[token] = args
		argsStashOrder = append(argsStashOrder, token)
		if len(argsStashOrder) > argsStashLimit {
			oldest := argsStashOrder[0]
			argsStashOrder = argsStashOrder[1:]
			delete(argsStash, oldest)
		}
	}
	return token
}

func shortHash(s string) string {
	h := fnv.New64a()
	_, _ = h.Write([]byte(s))
	return strconv.FormatUint(h.Sum64(), 36)
}
