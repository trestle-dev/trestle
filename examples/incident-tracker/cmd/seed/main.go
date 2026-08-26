package main

import (
	"bufio"
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/cookiejar"
	"os"
	"path/filepath"
	"strings"
	"time"
)

type client struct {
	base, csrf string
	http       *http.Client
}

type record struct {
	ID      string         `json:"id"`
	Version int            `json:"version"`
	Values  map[string]any `json:"values"`
}

func main() {
	base := flag.String("url", "http://127.0.0.1:8090", "Trestle base URL")
	verifyRestored := flag.Bool("verify-restored", false, "verify an offline-restored example")
	flag.Parse()
	jar, _ := cookiejar.New(nil)
	c := &client{base: strings.TrimRight(*base, "/"), http: &http.Client{Jar: jar, Timeout: 20 * time.Second}}
	c.waitReady()
	var session struct {
		CSRFToken string `json:"csrfToken"`
	}
	if *verifyRestored {
		c.json("POST", "/admin/v1/session", map[string]any{"email": "admin@example.com", "password": "mudblood"}, &session, 200)
		c.csrf = session.CSRFToken
		var incidents struct {
			Items []record `json:"items"`
		}
		c.json("GET", "/admin/v1/data/incidents/records?limit=100", nil, &incidents, 200)
		if len(incidents.Items) != 3 {
			fatalf("restored incident count is %d, want 3", len(incidents.Items))
		}
		var files struct {
			Items []json.RawMessage `json:"items"`
		}
		c.json("GET", "/admin/v1/files", nil, &files, 200)
		if len(files.Items) != 1 {
			fatalf("restored file count is %d, want 1", len(files.Items))
		}
		fmt.Println("Offline restore verified: administrator login, 3 incidents and 1 local file")
		return
	}
	c.json("POST", "/admin/v1/setup", map[string]any{"email": "admin@example.com", "password": "mudblood"}, &session, 200)
	c.csrf = session.CSRFToken

	createCollection(c, "incidents", []map[string]any{
		{"name": "title", "type": "text", "required": true},
		{"name": "severity", "type": "select", "required": true},
		{"name": "status", "type": "select", "required": true, "default": "open"},
		{"name": "summary", "type": "text", "required": true},
		{"name": "reporter_id", "type": "relation", "required": true},
		{"name": "resolved_at", "type": "datetime"},
		{"name": "context", "type": "json", "default": map[string]any{}},
	})
	createCollection(c, "updates", []map[string]any{
		{"name": "incident_id", "type": "relation", "required": true},
		{"name": "author_id", "type": "relation", "required": true},
		{"name": "kind", "type": "select", "required": true},
		{"name": "message", "type": "text", "required": true},
	})

	for _, collection := range []string{"incidents", "updates"} {
		rules := map[string]string{"list": `actor.kind == "user"`, "view": `actor.kind == "user"`, "create": `actor.id == input.` + map[string]string{"incidents": "reporter_id", "updates": "author_id"}[collection], "update": `actor.id == record.` + map[string]string{"incidents": "reporter_id", "updates": "author_id"}[collection], "delete": "false"}
		c.json("PUT", "/admin/v1/collection-rules/"+collection, map[string]any{"rules": rules}, nil, 200)
	}

	var registered struct {
		ID string `json:"id"`
	}
	c.publicJSON("POST", "/api/v1/auth/register", map[string]any{"email": "reporter@example.com", "password": "reporter7"}, &registered, 201, "")
	var login struct {
		AccessToken  string `json:"accessToken"`
		RefreshToken string `json:"refreshToken"`
	}
	c.publicJSON("POST", "/api/v1/auth/login", map[string]any{"email": "reporter@example.com", "password": "reporter7"}, &login, 200, "")

	var first record
	c.publicJSON("POST", "/api/v1/collections/incidents/records", map[string]any{"values": map[string]any{"title": "API latency elevated", "severity": "sev2", "status": "open", "summary": "The public API p95 crossed the operational threshold.", "reporter_id": registered.ID, "context": map[string]any{"region": "ap-southeast-2", "source": "synthetic-check"}}}, &first, 201, login.AccessToken)
	var second record
	c.publicJSON("POST", "/api/v1/collections/incidents/records", map[string]any{"values": map[string]any{"title": "Webhook delivery backlog", "severity": "sev3", "status": "monitoring", "summary": "Delivery retries increased after a downstream deployment.", "reporter_id": registered.ID, "context": map[string]any{"owner": "platform"}}}, &second, 201, login.AccessToken)
	for _, update := range []map[string]any{
		{"incident_id": first.ID, "author_id": registered.ID, "kind": "detection", "message": "Synthetic checks detected sustained latency."},
		{"incident_id": first.ID, "author_id": registered.ID, "kind": "mitigation", "message": "Traffic was shifted away from the affected worker pool."},
		{"incident_id": second.ID, "author_id": registered.ID, "kind": "note", "message": "Downstream recovery is being monitored."},
	} {
		c.publicJSON("POST", "/api/v1/collections/updates/records", map[string]any{"values": update}, nil, 201, login.AccessToken)
	}

	var credential struct {
		Secret string `json:"secret"`
	}
	c.json("POST", "/admin/v1/credentials", map[string]any{"name": "incident-exporter", "kind": "service", "scopes": []string{"records:read", "files:read"}}, &credential, 201)
	var listed struct {
		Items []record `json:"items"`
	}
	c.publicJSON("GET", "/api/v1/collections/incidents/records?limit=10", nil, &listed, 200, credential.Secret)
	if len(listed.Items) != 2 {
		fatalf("service credential saw %d incidents, want 2", len(listed.Items))
	}

	fileID := c.upload(first.ID)
	c.download(fileID, credential.Secret)
	c.replayEvent(credential.Secret)

	var webhook struct {
		ID string `json:"id"`
	}
	c.json("POST", "/admin/v1/webhooks", map[string]any{"name": "operations receiver", "url": "https://receiver.invalid/trestle-dogfood", "topics": []string{"record.created", "record.updated"}}, &webhook, 201)
	c.json("POST", "/admin/v1/webhooks/"+webhook.ID, map[string]any{"action": "disable"}, nil, 204)
	var function struct {
		ID string `json:"id"`
	}
	c.json("POST", "/admin/v1/functions", map[string]any{"name": "incident enricher", "target": "arn:aws:lambda:us-east-1:123456789012:function:incident-enricher", "region": "us-east-1", "topics": []string{"record.created"}, "callbackScopes": []string{"records:read"}}, &function, 201)
	// Generate one durable Lambda job. With no example AWS credentials it enters
	// the normal retry path, proving that provider diagnostics remain inspectable.
	var probe record
	c.publicJSON("POST", "/api/v1/collections/incidents/records", map[string]any{"values": map[string]any{"title": "Dogfood automation probe", "severity": "sev3", "status": "closed", "summary": "Created to exercise durable Lambda delivery without shipping credentials.", "reporter_id": registered.ID, "context": map[string]any{"dogfood": true}}}, &probe, 201, login.AccessToken)
	time.Sleep(1200 * time.Millisecond)
	c.json("POST", "/admin/v1/functions/"+function.ID, map[string]any{"action": "disable"}, nil, 204)
	c.json("POST", "/admin/v1/jobs", map[string]any{"kind": "noop", "payload": map[string]any{"source": "cp21"}, "idempotencyKey": "cp21-noop"}, nil, 201)
	time.Sleep(700 * time.Millisecond)
	var jobs struct {
		Items []json.RawMessage `json:"items"`
	}
	c.json("GET", "/admin/v1/jobs", nil, &jobs, 200)
	if len(jobs.Items) < 2 {
		fatalf("expected durable jobs, got %d", len(jobs.Items))
	}
	var audit struct {
		Items []json.RawMessage `json:"items"`
	}
	c.json("GET", "/admin/v1/audit", nil, &audit, 200)
	if len(audit.Items) == 0 {
		fatalf("audit trail is empty")
	}

	var backup struct {
		ID string `json:"id"`
	}
	c.json("POST", "/admin/v1/backups", map[string]any{}, &backup, 201)
	c.preflight(backup.ID)
	c.publicJSON("POST", "/api/v1/auth/logout", map[string]any{"refreshToken": login.RefreshToken}, nil, 204, "")

	fmt.Printf("Dogfood verified: 3 incidents, 3 related updates, file %s, realtime replay, %d audit facts, %d jobs, backup %s\n", fileID, len(audit.Items), len(jobs.Items), backup.ID)
}

func createCollection(c *client, name string, fields []map[string]any) {
	c.json("POST", "/admin/v1/collections", map[string]any{"name": name, "fields": fields}, nil, 201)
}

func (c *client) waitReady() {
	for deadline := time.Now().Add(15 * time.Second); time.Now().Before(deadline); time.Sleep(150 * time.Millisecond) {
		response, err := c.http.Get(c.base + "/system/ready")
		if err == nil {
			response.Body.Close()
			if response.StatusCode == 200 {
				return
			}
		}
	}
	fatalf("Trestle did not become ready")
}

func (c *client) json(method, path string, input, output any, want int) {
	c.doJSON(method, path, input, output, want, "", true)
}
func (c *client) publicJSON(method, path string, input, output any, want int, bearer string) {
	c.doJSON(method, path, input, output, want, bearer, false)
}
func (c *client) doJSON(method, path string, input, output any, want int, bearer string, admin bool) {
	var body io.Reader
	if input != nil {
		encoded, _ := json.Marshal(input)
		body = bytes.NewReader(encoded)
	}
	request, _ := http.NewRequest(method, c.base+path, body)
	request.Header.Set("Accept", "application/json")
	if input != nil {
		request.Header.Set("Content-Type", "application/json")
	}
	if bearer != "" {
		request.Header.Set("Authorization", "Bearer "+bearer)
	}
	if admin {
		request.Header.Set("Origin", c.base)
		if c.csrf != "" {
			request.Header.Set("X-Trestle-CSRF", c.csrf)
		}
	}
	response, err := c.http.Do(request)
	if err != nil {
		fatalf("%s %s: %v", method, path, err)
	}
	defer response.Body.Close()
	payload, _ := io.ReadAll(response.Body)
	if response.StatusCode != want {
		fatalf("%s %s: got %d, want %d: %s", method, path, response.StatusCode, want, payload)
	}
	if output != nil && len(payload) > 0 && json.Unmarshal(payload, output) != nil {
		fatalf("decode %s: %s", path, payload)
	}
}

func (c *client) upload(recordID string) string {
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	part, _ := writer.CreateFormFile("file", "latency-runbook.md")
	_, _ = io.WriteString(part, "# Latency runbook\n\n1. Confirm scope.\n2. Shift traffic.\n3. Preserve evidence.\n")
	_ = writer.WriteField("collection", "incidents")
	_ = writer.WriteField("recordId", recordID)
	_ = writer.Close()
	request, _ := http.NewRequest("POST", c.base+"/api/v1/files", &body)
	request.Header.Set("Content-Type", writer.FormDataContentType())
	request.Header.Set("Origin", c.base)
	request.Header.Set("X-Trestle-CSRF", c.csrf)
	response, err := c.http.Do(request)
	if err != nil {
		fatalf("upload: %v", err)
	}
	defer response.Body.Close()
	var result struct {
		ID string `json:"id"`
	}
	if response.StatusCode != 201 || json.NewDecoder(response.Body).Decode(&result) != nil {
		fatalf("upload failed: %s", response.Status)
	}
	return result.ID
}

func (c *client) download(id, token string) {
	request, _ := http.NewRequest("GET", c.base+"/api/v1/files/"+id, nil)
	request.Header.Set("Authorization", "Bearer "+token)
	response, err := c.http.Do(request)
	if err != nil || response.StatusCode != 200 {
		fatalf("download failed")
	}
	payload, _ := io.ReadAll(response.Body)
	response.Body.Close()
	if !bytes.Contains(payload, []byte("Latency runbook")) {
		fatalf("download content mismatch")
	}
}

func (c *client) replayEvent(token string) {
	request, _ := http.NewRequest("GET", c.base+"/api/v1/realtime?topic=record.created", nil)
	request.Header.Set("Authorization", "Bearer "+token)
	request.Header.Set("Last-Event-ID", "0")
	ctx := request.Context()
	_ = ctx
	response, err := c.http.Do(request)
	if err != nil || response.StatusCode != 200 {
		fatalf("realtime replay failed")
	}
	defer response.Body.Close()
	scanner := bufio.NewScanner(response.Body)
	for scanner.Scan() {
		if strings.HasPrefix(scanner.Text(), "data: ") {
			return
		}
	}
	fatalf("realtime replay returned no event")
}

func (c *client) preflight(id string) {
	response, err := c.http.Get(c.base + "/admin/v1/backups/" + id)
	if err != nil || response.StatusCode != 200 {
		fatalf("backup download failed")
	}
	archive, _ := io.ReadAll(response.Body)
	response.Body.Close()
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	part, _ := writer.CreateFormFile("backup", filepath.Base(id))
	_, _ = part.Write(archive)
	_ = writer.Close()
	request, _ := http.NewRequest("POST", c.base+"/admin/v1/restores/preflight", &body)
	request.Header.Set("Content-Type", writer.FormDataContentType())
	request.Header.Set("Origin", c.base)
	request.Header.Set("X-Trestle-CSRF", c.csrf)
	result, err := c.http.Do(request)
	if err != nil {
		fatalf("preflight: %v", err)
	}
	defer result.Body.Close()
	var checked struct {
		Valid bool `json:"valid"`
	}
	_ = json.NewDecoder(result.Body).Decode(&checked)
	if result.StatusCode != 200 || !checked.Valid {
		fatalf("backup preflight failed")
	}
}

func fatalf(format string, values ...any) { fmt.Fprintf(os.Stderr, format+"\n", values...); os.Exit(1) }
