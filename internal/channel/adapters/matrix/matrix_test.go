package matrix

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/memohai/memoh/internal/channel"
)

func TestIsMatrixBotMentionedByMentionsMetadata(t *testing.T) {
	content := map[string]any{
		"body": "hi bot",
		"m.mentions": map[string]any{
			"user_ids": []any{"@memoh:example.com"},
		},
	}
	if !isMatrixBotMentioned("@memoh:example.com", content) {
		t.Fatal("expected mention metadata to be detected")
	}
}

func TestIsMatrixBotMentionedByFormattedBody(t *testing.T) {
	content := map[string]any{
		"body":           "hello Memoh",
		"formatted_body": `<a href="https://matrix.to/#/@memoh:example.com">Memoh</a> hello`,
	}
	if !isMatrixBotMentioned("@memoh:example.com", content) {
		t.Fatal("expected formatted body mention to be detected")
	}
}

func TestIsMatrixBotMentionedByBodyFallback(t *testing.T) {
	content := map[string]any{
		"body": "@memoh:example.com ping",
	}
	if !isMatrixBotMentioned("@memoh:example.com", content) {
		t.Fatal("expected body fallback mention to be detected")
	}
}

func TestMatrixSinceTokenFromRouting(t *testing.T) {
	routing := map[string]any{
		matrixRoutingStateKey: map[string]any{"since_token": "s123"},
	}
	if got := matrixSinceTokenFromRouting(routing); got != "s123" {
		t.Fatalf("unexpected since token: %q", got)
	}
}

func TestPersistSinceTokenUsesConfiguredSaver(t *testing.T) {
	var gotConfigID string
	var gotSince string
	adapter := NewMatrixAdapter(nil)
	adapter.SetSyncStateSaver(func(_ context.Context, configID string, since string) error {
		gotConfigID = configID
		gotSince = since
		return nil
	})
	if err := adapter.persistSinceToken(context.Background(), "cfg-1", "token-1"); err != nil {
		t.Fatalf("persistSinceToken returned error: %v", err)
	}
	if gotConfigID != "cfg-1" || gotSince != "token-1" {
		t.Fatalf("unexpected saver args: %q %q", gotConfigID, gotSince)
	}
}

func TestBootstrapSinceTokenPersistsLatestCursor(t *testing.T) {
	adapter := NewMatrixAdapter(nil)
	adapter.httpClient = &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
		return &http.Response{
			StatusCode: http.StatusOK,
			Body:       io.NopCloser(strings.NewReader(`{"next_batch":"s123","rooms":{"join":{"!room:example.com":{"timeline":{"events":[{"event_id":"$evt1"}]}}}}}`)),
			Header:     make(http.Header),
		}, nil
	})}
	var gotConfigID string
	var gotSince string
	adapter.SetSyncStateSaver(func(_ context.Context, configID string, since string) error {
		gotConfigID = configID
		gotSince = since
		return nil
	})

	since, err := adapter.bootstrapSinceToken(context.Background(), channel.ChannelConfig{ID: "cfg-1"}, Config{
		HomeserverURL: "https://matrix.example.com",
		AccessToken:   "tok",
	})
	if err != nil {
		t.Fatalf("bootstrapSinceToken returned error: %v", err)
	}
	if since != "s123" {
		t.Fatalf("unexpected since token: %q", since)
	}
	if gotConfigID != "cfg-1" || gotSince != "s123" {
		t.Fatalf("unexpected persisted cursor: %q %q", gotConfigID, gotSince)
	}
	if !adapter.seenEvent("cfg-1", "$evt1") {
		t.Fatal("expected bootstrap event to be remembered as seen")
	}
}

func TestBuildMatrixMessageContentIncludesFormattedHTMLForMarkdown(t *testing.T) {
	content := buildMatrixMessageContent(channel.Message{
		Text:   "**bold**\n\n- item",
		Format: channel.MessageFormatMarkdown,
	}, false, "")

	if got := content["body"]; got != "**bold**\n\n- item" {
		t.Fatalf("unexpected body: %#v", got)
	}
	if got := content["format"]; got != matrixHTMLFormat {
		t.Fatalf("unexpected format: %#v", got)
	}
	html, ok := content["formatted_body"].(string)
	if !ok || !strings.Contains(html, "<strong>bold</strong>") || !strings.Contains(html, "<ul>") {
		t.Fatalf("unexpected formatted body: %#v", content["formatted_body"])
	}
}

func TestBuildMatrixMessageContentAddsFormattedHTMLToEdits(t *testing.T) {
	content := buildMatrixMessageContent(channel.Message{
		Text:   "`code`",
		Format: channel.MessageFormatMarkdown,
	}, true, "$evt1")

	newContent, ok := content["m.new_content"].(map[string]any)
	if !ok {
		t.Fatalf("expected m.new_content map, got %#v", content["m.new_content"])
	}
	if got := newContent["format"]; got != matrixHTMLFormat {
		t.Fatalf("unexpected edit format: %#v", got)
	}
	html, ok := newContent["formatted_body"].(string)
	if !ok || !strings.Contains(html, "<code>code</code>") {
		t.Fatalf("unexpected edit formatted body: %#v", newContent["formatted_body"])
	}
}

func TestStripMatrixReplyFallback(t *testing.T) {
	body := "> <@memoh:example.com> This looks like Antelope Canyon\n>\nWhere is Antelope Canyon?"
	if got := stripMatrixReplyFallback(body); got != "Where is Antelope Canyon?" {
		t.Fatalf("unexpected stripped body: %q", got)
	}
}

func TestMatrixHandleEventExpandsRepliedImageContext(t *testing.T) {
	adapter := NewMatrixAdapter(nil)
	adapter.rememberRoomConversationType("cfg-1", "!room:example.com", "group")
	adapter.httpClient = &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
		if !strings.Contains(req.URL.Path, "/rooms/!room:example.com/event/$img1") {
			t.Fatalf("unexpected request path: %s", req.URL.Path)
		}
		return &http.Response{
			StatusCode: http.StatusOK,
			Body: io.NopCloser(strings.NewReader(`{
				"event_id":"$img1",
				"type":"m.room.message",
				"sender":"@memoh:example.com",
				"unsigned":{"displayname":"Memoh"},
				"content":{
					"msgtype":"m.image",
					"body":"canyon.jpg",
					"url":"mxc://matrix.example.com/media123",
					"info":{"mimetype":"image/jpeg","w":640,"h":480}
				}
			}`)),
			Header: make(http.Header),
		}, nil
	})}

	var captured channel.InboundMessage
	delivered, err := adapter.handleEvent(
		context.Background(),
		channel.ChannelConfig{ID: "cfg-1", BotID: "bot-1"},
		Config{HomeserverURL: "https://matrix.example.com", AccessToken: "tok", UserID: "@memoh:example.com"},
		matrixEvent{
			EventID: "$evt2",
			Type:    "m.room.message",
			Sender:  "@alex:example.com",
			RoomID:  "!room:example.com",
			Content: map[string]any{
				"msgtype": "m.text",
				"body":    "> <@memoh:example.com> photo\n>\nWhere is Antelope Canyon?",
				"m.relates_to": map[string]any{
					"m.in_reply_to": map[string]any{"event_id": "$img1"},
				},
			},
		},
		func(_ context.Context, _ channel.ChannelConfig, msg channel.InboundMessage) error {
			captured = msg
			return nil
		},
	)
	if err != nil {
		t.Fatalf("handleEvent returned error: %v", err)
	}
	if !delivered {
		t.Fatal("expected event to be delivered")
	}
	if got := captured.Message.Text; got != "[Reply to Memoh: [image]]\nWhere is Antelope Canyon?" {
		t.Fatalf("unexpected message text: %q", got)
	}
	if len(captured.Message.Attachments) != 1 {
		t.Fatalf("expected one quoted attachment, got %d", len(captured.Message.Attachments))
	}
	if captured.Message.Attachments[0].PlatformKey != "mxc://matrix.example.com/media123" {
		t.Fatalf("unexpected quoted attachment: %#v", captured.Message.Attachments[0])
	}
	isReplyToBot, _ := captured.Metadata["is_reply_to_bot"].(bool)
	if !isReplyToBot {
		t.Fatalf("expected is_reply_to_bot metadata to be true")
	}
	if rawText, _ := captured.Metadata["raw_text"].(string); rawText != "Where is Antelope Canyon?" {
		t.Fatalf("unexpected raw_text metadata: %q", rawText)
	}
}

func TestMatrixHandleEventMarksDirectConversationFromJoinedMembers(t *testing.T) {
	joinedMembersRequests := 0
	adapter := NewMatrixAdapter(nil)
	adapter.httpClient = &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
		switch req.URL.Path {
		case "/_matrix/client/v3/rooms/!room:example.com/joined_members":
			joinedMembersRequests++
			return &http.Response{
				StatusCode: http.StatusOK,
				Body: io.NopCloser(strings.NewReader(`{
					"joined": {
						"@alex:example.com": {"display_name": "Alex"},
						"@memoh:example.com": {"display_name": "Memoh"}
					}
				}`)),
				Header: make(http.Header),
			}, nil
		default:
			t.Fatalf("unexpected request path: %s", req.URL.Path)
			return nil, nil
		}
	})}

	var captured []channel.InboundMessage
	for i := 0; i < 2; i++ {
		delivered, err := adapter.handleEvent(
			context.Background(),
			channel.ChannelConfig{ID: "cfg-1", BotID: "bot-1"},
			Config{HomeserverURL: "https://matrix.example.com", AccessToken: "tok", UserID: "@memoh:example.com"},
			matrixEvent{
				EventID: fmt.Sprintf("$evt%d", i+1),
				Type:    "m.room.message",
				Sender:  "@alex:example.com",
				RoomID:  "!room:example.com",
				Content: map[string]any{
					"msgtype": "m.text",
					"body":    "ping",
				},
			},
			func(_ context.Context, _ channel.ChannelConfig, msg channel.InboundMessage) error {
				captured = append(captured, msg)
				return nil
			},
		)
		if err != nil {
			t.Fatalf("handleEvent returned error: %v", err)
		}
		if !delivered {
			t.Fatal("expected event to be delivered")
		}
	}

	if len(captured) != 2 {
		t.Fatalf("expected two captured messages, got %d", len(captured))
	}
	if captured[0].Conversation.Type != "direct" {
		t.Fatalf("expected direct conversation type, got %q", captured[0].Conversation.Type)
	}
	if joinedMembersRequests != 1 {
		t.Fatalf("expected joined_members lookup to be cached, got %d requests", joinedMembersRequests)
	}
}

func TestExtractMatrixDirectRoomIDs(t *testing.T) {
	roomIDs := extractMatrixDirectRoomIDs(matrixSyncResponse{
		AccountData: struct {
			Events []matrixSyncEvent `json:"events"`
		}{
			Events: []matrixSyncEvent{{
				Type: "m.direct",
				Content: map[string]any{
					"@alice:example.com": []any{"!dm:example.com", " !dm2:example.com "},
				},
			}},
		},
	})

	if _, ok := roomIDs["!dm:example.com"]; !ok {
		t.Fatal("expected first direct room id to be extracted")
	}
	if _, ok := roomIDs["!dm2:example.com"]; !ok {
		t.Fatal("expected second direct room id to be extracted")
	}
}

func TestExtractMatrixInboundContentParsesImageAttachment(t *testing.T) {
	text, attachments := extractMatrixInboundContent(map[string]any{
		"msgtype": "m.image",
		"body":    "diagram.png",
		"url":     "mxc://matrix.example.com/media123",
		"info": map[string]any{
			"mimetype": "image/png",
			"size":     42,
			"w":        640,
			"h":        480,
		},
	})
	if text != "" {
		t.Fatalf("expected empty text for attachment message, got %q", text)
	}
	if len(attachments) != 1 {
		t.Fatalf("expected 1 attachment, got %d", len(attachments))
	}
	att := attachments[0]
	if att.Type != channel.AttachmentImage {
		t.Fatalf("unexpected attachment type: %s", att.Type)
	}
	if att.PlatformKey != "mxc://matrix.example.com/media123" {
		t.Fatalf("unexpected platform key: %q", att.PlatformKey)
	}
	if att.Name != "diagram.png" || att.Mime != "image/png" {
		t.Fatalf("unexpected attachment metadata: %#v", att)
	}
	if att.Width != 640 || att.Height != 480 || att.Size != 42 {
		t.Fatalf("unexpected attachment dimensions: %#v", att)
	}
}

func TestMatrixSendUploadsBase64AttachmentAndSendsMediaEvent(t *testing.T) {
	requests := make([]string, 0, 2)
	uploadedContentTypes := make([]string, 0, 1)
	adapter := NewMatrixAdapter(nil)
	adapter.httpClient = &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
		requests = append(requests, req.URL.Path)
		if strings.Contains(req.URL.Path, "/_matrix/media/v3/upload") {
			uploadedContentTypes = append(uploadedContentTypes, req.Header.Get("Content-Type"))
			return &http.Response{
				StatusCode: http.StatusOK,
				Body:       io.NopCloser(strings.NewReader(`{"content_uri":"mxc://matrix.example.com/uploaded1"}`)),
				Header:     make(http.Header),
			}, nil
		}
		payload, err := io.ReadAll(req.Body)
		if err != nil {
			return nil, err
		}
		var content map[string]any
		if err := json.Unmarshal(payload, &content); err != nil {
			return nil, err
		}
		if got := content["msgtype"]; got != "m.image" {
			t.Fatalf("unexpected msgtype: %#v", got)
		}
		if got := content["url"]; got != "mxc://matrix.example.com/uploaded1" {
			t.Fatalf("unexpected uploaded uri: %#v", got)
		}
		return &http.Response{
			StatusCode: http.StatusOK,
			Body:       io.NopCloser(strings.NewReader(`{"event_id":"$evt1"}`)),
			Header:     make(http.Header),
		}, nil
	})}

	err := adapter.Send(context.Background(), channel.ChannelConfig{
		BotID: "bot-1",
		Credentials: map[string]any{
			"homeserverUrl": "https://matrix.example.com",
			"userId":        "@memoh:example.com",
			"accessToken":   "tok",
		},
	}, channel.OutboundMessage{
		Target: "!room:example.com",
		Message: channel.Message{
			Attachments: []channel.Attachment{{
				Type:   channel.AttachmentImage,
				Name:   "chart.png",
				Mime:   "image/png",
				Base64: "data:image/png;base64,aGVsbG8=",
			}},
		},
	})
	if err != nil {
		t.Fatalf("send returned error: %v", err)
	}
	if len(requests) != 2 {
		t.Fatalf("expected upload and send requests, got %d", len(requests))
	}
	if len(uploadedContentTypes) != 1 || uploadedContentTypes[0] != "image/png" {
		t.Fatalf("unexpected upload content type: %#v", uploadedContentTypes)
	}
}

func TestMatrixResolveAttachmentDownloadsMXC(t *testing.T) {
	adapter := NewMatrixAdapter(nil)
	adapter.httpClient = &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
		if !strings.Contains(req.URL.Path, "/_matrix/client/v1/media/download/matrix.example.com/media123/image.png") {
			t.Fatalf("unexpected download path: %s", req.URL.Path)
		}
		resp := &http.Response{
			StatusCode: http.StatusOK,
			Body:       io.NopCloser(strings.NewReader("file-bytes")),
			Header:     make(http.Header),
		}
		resp.Header.Set("Content-Type", "image/png")
		resp.ContentLength = int64(len("file-bytes"))
		return resp, nil
	})}

	payload, err := adapter.ResolveAttachment(context.Background(), channel.ChannelConfig{
		Credentials: map[string]any{
			"homeserverUrl": "https://matrix.example.com",
			"userId":        "@memoh:example.com",
			"accessToken":   "tok",
		},
	}, channel.Attachment{
		PlatformKey: "mxc://matrix.example.com/media123",
		Name:        "image.png",
	})
	if err != nil {
		t.Fatalf("ResolveAttachment returned error: %v", err)
	}
	defer func() { _ = payload.Reader.Close() }()
	data, err := io.ReadAll(payload.Reader)
	if err != nil {
		t.Fatalf("read payload: %v", err)
	}
	if string(data) != "file-bytes" {
		t.Fatalf("unexpected payload: %q", string(data))
	}
	if payload.Mime != "image/png" || payload.Name != "image.png" || payload.Size != int64(len("file-bytes")) {
		t.Fatalf("unexpected payload metadata: %#v", payload)
	}
}

func TestMatrixResolveAttachmentFallsBackToLegacyMediaDownload(t *testing.T) {
	paths := make([]string, 0, 2)
	adapter := NewMatrixAdapter(nil)
	adapter.httpClient = &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
		paths = append(paths, req.URL.Path)
		if strings.Contains(req.URL.Path, "/_matrix/client/v1/media/download/") {
			return &http.Response{
				StatusCode: http.StatusNotFound,
				Body:       io.NopCloser(strings.NewReader(`{"errcode":"M_NOT_FOUND"}`)),
				Header:     make(http.Header),
			}, nil
		}
		if !strings.Contains(req.URL.Path, "/_matrix/media/v3/download/matrix.example.com/media123") {
			t.Fatalf("unexpected fallback path: %s", req.URL.Path)
		}
		resp := &http.Response{
			StatusCode: http.StatusOK,
			Body:       io.NopCloser(strings.NewReader("legacy-file")),
			Header:     make(http.Header),
		}
		resp.Header.Set("Content-Type", "application/octet-stream")
		return resp, nil
	})}

	payload, err := adapter.ResolveAttachment(context.Background(), channel.ChannelConfig{
		Credentials: map[string]any{
			"homeserverUrl": "https://matrix.example.com",
			"userId":        "@memoh:example.com",
			"accessToken":   "tok",
		},
	}, channel.Attachment{
		PlatformKey: "mxc://matrix.example.com/media123",
	})
	if err != nil {
		t.Fatalf("ResolveAttachment returned error: %v", err)
	}
	defer func() { _ = payload.Reader.Close() }()
	if len(paths) != 2 {
		t.Fatalf("expected authenticated and legacy download attempts, got %d", len(paths))
	}
}
