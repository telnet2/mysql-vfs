package main

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestHandlePrincipalsDefaultPath(t *testing.T) {
	var requestedPath, headerActor string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/policies/resolve" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		requestedPath = r.URL.Query().Get("path")
		headerActor = r.Header.Get(actorHeader)
		w.Header().Set("Content-Type", "application/json")
		io.WriteString(w, `{"directory_id":"dir1","manifests":[],"principals":{"users":[{"id":"alice"}]}}`)
	}))
	defer server.Close()

	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	c := &cli{
		metadata: &metadataClient{baseURL: server.URL, httpClient: server.Client(), actor: "alice"},
		content:  &contentClient{},
		cwdDir:   directoryDTO{Path: "/"},
		dirCache: make(map[string]directoryDTO),
		stdout:   stdout,
		stderr:   stderr,
		actor:    "alice",
	}

	c.handlePrincipals([]string{"principals"})

	if requestedPath != "/" {
		t.Fatalf("expected path '/', got %q", requestedPath)
	}
	if headerActor != "alice" {
		t.Fatalf("expected actor header alice, got %q", headerActor)
	}
	if stderr.Len() != 0 {
		t.Fatalf("unexpected stderr: %s", stderr.String())
	}

	var payload struct {
		DirectoryID string           `json:"directory_id"`
		Users       []map[string]any `json:"users"`
		Groups      []map[string]any `json:"groups"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &payload); err != nil {
		t.Fatalf("unmarshal output: %v", err)
	}
	if payload.DirectoryID != "dir1" {
		t.Fatalf("expected directory id 'dir1', got %q", payload.DirectoryID)
	}
	if len(payload.Users) != 1 || payload.Users[0]["id"] != "alice" {
		t.Fatalf("unexpected users payload: %#v", payload.Users)
	}
}

func TestHandlePrincipalsWithTypeFilter(t *testing.T) {
	var requestedType, requestedPath, headerActor string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/policies/resolve" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		requestedType = r.URL.Query().Get("type")
		requestedPath = r.URL.Query().Get("path")
		headerActor = r.Header.Get(actorHeader)
		w.Header().Set("Content-Type", "application/json")
		io.WriteString(w, `{"directory_id":"dir2","manifests":[],"principals":{"groups":[{"id":"admins"}]}}`)
	}))
	defer server.Close()

	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	c := &cli{
		metadata: &metadataClient{baseURL: server.URL, httpClient: server.Client(), actor: "carol"},
		content:  &contentClient{},
		cwdDir:   directoryDTO{Path: "/"},
		dirCache: make(map[string]directoryDTO),
		stdout:   stdout,
		stderr:   stderr,
		actor:    "carol",
	}

	c.handlePrincipals([]string{"principals", "--type", "user", "/docs"})

	if requestedType != "user" {
		t.Fatalf("expected type 'user', got %q", requestedType)
	}
	if requestedPath != "/docs" {
		t.Fatalf("expected path '/docs', got %q", requestedPath)
	}
	if headerActor != "carol" {
		t.Fatalf("expected actor header carol, got %q", headerActor)
	}
	if stderr.Len() != 0 {
		t.Fatalf("unexpected stderr: %s", stderr.String())
	}

	var payload struct {
		DirectoryID string           `json:"directory_id"`
		Users       []map[string]any `json:"users"`
		Groups      []map[string]any `json:"groups"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &payload); err != nil {
		t.Fatalf("unmarshal output: %v", err)
	}
	if payload.DirectoryID != "dir2" {
		t.Fatalf("expected directory id 'dir2', got %q", payload.DirectoryID)
	}
	if len(payload.Groups) != 1 || payload.Groups[0]["id"] != "admins" {
		t.Fatalf("unexpected groups payload: %#v", payload.Groups)
	}
}

func TestEnsurePolicyAdminAllows(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		io.WriteString(w, `{"directory_id":"dir1","manifests":[],"principals":{"users":[{"id":"alice","groups":["admin"]}]}}`)
	}))
	defer server.Close()
	cli := &cli{
		metadata: &metadataClient{baseURL: server.URL, httpClient: server.Client(), actor: "alice"},
		actor:    "alice",
	}
	if err := cli.ensurePolicyAdmin("/docs"); err != nil {
		t.Fatalf("expected admin access, got %v", err)
	}
}

func TestEnsurePolicyAdminBlocks(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		io.WriteString(w, `{"directory_id":"dir1","manifests":[],"principals":{"groups":[{"id":"admins","members":["eve"]}]}}`)
	}))
	defer server.Close()
	cli := &cli{
		metadata: &metadataClient{baseURL: server.URL, httpClient: server.Client(), actor: "bob"},
		actor:    "bob",
	}
	if err := cli.ensurePolicyAdmin("/docs"); err == nil {
		t.Fatalf("expected error for non-admin actor")
	}
}
