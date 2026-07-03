package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"math/rand/v2"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"strings"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/labstack/echo/v4"

	"github.com/memohai/memoh/internal/accounts"
	"github.com/memohai/memoh/internal/bots"
	postgresqlc "github.com/memohai/memoh/internal/db/postgres/sqlc"
	postgresstore "github.com/memohai/memoh/internal/db/postgres/store"
	"github.com/memohai/memoh/internal/handlers"
	"github.com/memohai/memoh/internal/message"
	"github.com/memohai/memoh/internal/session"
	"github.com/memohai/memoh/internal/toolapproval"
	"github.com/memohai/memoh/internal/userinput"
)

type httpExecutor struct {
	cfg     Config
	echo    *echo.Echo
	handler *handlers.MessageHandler
}

func newHTTPExecutor(cfg Config, pool *pgxpool.Pool) *httpExecutor {
	logger := slog.New(slog.DiscardHandler)
	sqlcQueries := postgresqlc.New(pool)
	queries := postgresstore.NewQueriesWithPool(pool, sqlcQueries)
	accountStore := postgresstore.NewWithPool(pool, sqlcQueries)

	messageService := message.NewService(logger, queries)
	sessionService := session.NewService(logger, queries, nil)
	botService := bots.NewService(logger, queries)
	accountService := accounts.NewService(logger, accountStore)
	handler := handlers.NewMessageHandler(logger, nil, messageService, sessionService, botService, accountService)
	handler.SetToolApprovalService(toolapproval.NewService(logger, queries, nil))
	handler.SetUserInputService(userinput.NewService(logger, queries))

	e := echo.New()
	e.HideBanner = true
	e.HidePort = true
	e.Logger.SetOutput(io.Discard)

	return &httpExecutor{
		cfg:     cfg,
		echo:    e,
		handler: handler,
	}
}

func (*httpExecutor) querySource() string {
	return querySourceHTTPHandler
}

func (*httpExecutor) scanMode() string {
	return scanModeHTTPJSON
}

func (e *httpExecutor) execQuery(ctx context.Context, queryName string, s SessionSeed, rng *rand.Rand) (int64, error) {
	switch queryName {
	case queryChatPageUI:
		return e.execListMessages(ctx, s, rng, httpListMessagesLatest)
	case queryLatestPage:
		return e.execListMessages(ctx, s, rng, httpListMessagesLatest)
	case queryBeforePage:
		return e.execListMessages(ctx, s, rng, httpListMessagesBefore)
	case queryLocateWindow:
		return e.execLocateMessages(ctx, s, rng, true)
	case queryExternalLookup:
		return e.execLocateMessages(ctx, s, rng, false)
	default:
		return 0, fmt.Errorf("http runner supports ListMessages/LocateMessage scenarios only, got %s", queryName)
	}
}

type httpListMessagesMode string

const (
	httpListMessagesLatest httpListMessagesMode = "latest"
	httpListMessagesBefore httpListMessagesMode = "before"
)

func (e *httpExecutor) execListMessages(ctx context.Context, s SessionSeed, rng *rand.Rand, mode httpListMessagesMode) (int64, error) {
	if s.OwnerUserID == uuid.Nil {
		return 0, errors.New("http runner requires owner_user_id in seed catalog; reseed or reload catalog")
	}
	values := url.Values{}
	values.Set("session_id", s.SessionID.String())
	values.Set("limit", strconv.Itoa(e.cfg.Workload.PageSize))
	if format := strings.TrimSpace(e.cfg.Workload.HTTPFormat); format != "" {
		values.Set("format", format)
	}
	headID := selectedHead(e.cfg, s, rng)
	if headID != uuid.Nil {
		values.Set("head_turn_id", headID.String())
	}
	if mode == httpListMessagesBefore {
		cursorID, cursorTime := selectedCursorForHead(s, headID, rng)
		values.Set("before_id", cursorID.String())
		values.Set("before", cursorTime.UTC().Format(httpTimeFormat))
	}

	//nolint:contextcheck // ctx is propagated through the httptest request in callMessageHandler.
	rec, err := e.callMessageHandler(ctx, http.MethodGet, "/bots/"+s.BotID.String()+"/messages?"+values.Encode(), s, func(c echo.Context) error {
		c.SetPath("/bots/:bot_id/messages")
		c.SetParamNames("bot_id")
		c.SetParamValues(s.BotID.String())
		return e.handler.ListMessages(c)
	})
	if err != nil {
		return 0, err
	}
	return e.countItems(rec.Body.Bytes())
}

func (e *httpExecutor) execLocateMessages(ctx context.Context, s SessionSeed, rng *rand.Rand, withWindow bool) (int64, error) {
	if s.OwnerUserID == uuid.Nil {
		return 0, errors.New("http runner requires owner_user_id in seed catalog; reseed or reload catalog")
	}
	externalID := selectedExternalMessageID(s, rng)
	if strings.TrimSpace(externalID) == "" {
		return 0, queryArgError("external_lookup requires external_message_id")
	}
	values := url.Values{}
	values.Set("session_id", s.SessionID.String())
	values.Set("external_message_id", externalID)
	if withWindow {
		values.Set("before", strconv.Itoa(e.cfg.Workload.PageSize))
		values.Set("after", strconv.Itoa(e.cfg.Workload.PageSize))
	} else {
		values.Set("before", "0")
		values.Set("after", "0")
	}
	if headID := selectedHead(e.cfg, s, rng); headID != uuid.Nil {
		values.Set("head_turn_id", headID.String())
	}

	//nolint:contextcheck // ctx is propagated through the httptest request in callMessageHandler.
	rec, err := e.callMessageHandler(ctx, http.MethodGet, "/bots/"+s.BotID.String()+"/messages/locate?"+values.Encode(), s, func(c echo.Context) error {
		c.SetPath("/bots/:bot_id/messages/locate")
		c.SetParamNames("bot_id")
		c.SetParamValues(s.BotID.String())
		return e.handler.LocateMessage(c)
	})
	if err != nil {
		return 0, err
	}
	return e.countItems(rec.Body.Bytes())
}

func (e *httpExecutor) callMessageHandler(ctx context.Context, method, target string, s SessionSeed, call func(echo.Context) error) (*httptest.ResponseRecorder, error) {
	req := httptest.NewRequest(method, target, nil).WithContext(ctx)
	req.Header.Set(echo.HeaderAccept, echo.MIMEApplicationJSON)
	rec := httptest.NewRecorder()
	c := e.echo.NewContext(req, rec)
	c.Set("user", &jwt.Token{
		Valid: true,
		Claims: jwt.MapClaims{
			"user_id": s.OwnerUserID.String(),
			"sub":     s.OwnerUserID.String(),
		},
	})

	if err := call(c); err != nil {
		if ctxErr := ctx.Err(); ctxErr != nil {
			return nil, ctxErr
		}
		return nil, normalizeHTTPHandlerError(err)
	}
	if ctxErr := ctx.Err(); ctxErr != nil {
		return nil, ctxErr
	}
	if rec.Code != http.StatusOK {
		body := strings.TrimSpace(rec.Body.String())
		if body == "" {
			body = http.StatusText(rec.Code)
		}
		return nil, fmt.Errorf("http status %d: %s", rec.Code, body)
	}
	return rec, nil
}

func (e *httpExecutor) countItems(body []byte) (int64, error) {
	if !e.cfg.Workload.HTTPDecodeJSON {
		return int64(len(body)), nil
	}
	dec := json.NewDecoder(bytes.NewReader(body))
	var response struct {
		Items []json.RawMessage `json:"items"`
	}
	if err := dec.Decode(&response); err != nil {
		return 0, fmt.Errorf("decode HTTP JSON response: %w", err)
	}
	return int64(len(response.Items)), nil
}

func normalizeHTTPHandlerError(err error) error {
	var httpErr *echo.HTTPError
	if errors.As(err, &httpErr) {
		return fmt.Errorf("http status %d: %v", httpErr.Code, httpErr.Message)
	}
	return err
}

const httpTimeFormat = "2006-01-02T15:04:05.999999999Z07:00"
