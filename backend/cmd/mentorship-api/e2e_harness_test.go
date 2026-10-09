// Copyright The Linux Foundation and each contributor to LFX.
// SPDX-License-Identifier: MIT

//go:build e2e

package main

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/rsa"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/johannesboyne/gofakes3"
	"github.com/johannesboyne/gofakes3/backend/s3mem"
	natsserver "github.com/nats-io/nats-server/v2/server"
	"github.com/nats-io/nats.go"
	"github.com/nats-io/nats.go/jetstream"
	"gopkg.in/go-jose/go-jose.v2"
)

const (
	e2eIssuer       = "heimdall-e2e"
	e2eAudience     = "lfx-mentorship-e2e"
	e2eKeyID        = "e2e-key"
	e2eInviteSecret = "e2e-invite-secret"
	e2eHRInbox      = "hr@e2e.example.org"
	e2eSiteURL      = "https://mentorship.e2e.example.org"
	e2eSelfServeURL = "https://selfserve.e2e.example.org"
	e2eCDNPrefix    = "https://cdn.e2e.example.org"
	e2eLogosBucket  = "e2e-logos"
	e2eFilesBucket  = "e2e-attachments"
	apiPrefix       = "/mentorship/v1"
)

// e2e is the process-wide environment: one API server and its fake dependencies, shared by every test.
var e2e *environment

type environment struct {
	baseURL string
	pool    *pgxpool.Pool
	key     *rsa.PrivateKey
	js      jetstream.JetStream
	fakes   *fakes
	s3      gofakes3.Backend
}

func TestMain(m *testing.M) {
	os.Exit(runE2E(m))
}

func runE2E(m *testing.M) int {
	dsn := os.Getenv("TEST_DATABASE_DSN")
	if dsn == "" {
		fmt.Fprintln(os.Stderr, "e2e: TEST_DATABASE_DSN is not set; skipping")
		return 0
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	env, cleanup, err := startEnvironment(ctx, dsn)
	if err != nil {
		fmt.Fprintf(os.Stderr, "e2e: %v\n", err)
		return 1
	}
	defer cleanup()
	e2e = env
	return m.Run()
}

func startEnvironment(ctx context.Context, dsn string) (*environment, func(), error) {
	var closers []func()
	cleanup := func() {
		for i := len(closers) - 1; i >= 0; i-- {
			closers[i]()
		}
	}
	fail := func(err error) (*environment, func(), error) {
		cleanup()
		return nil, nil, err
	}

	// The server reads its database and AWS settings from the environment, as it does in a pod.
	for _, key := range []string{"DB_HOST", "DB_PORT", "DB_USER", "DB_PASSWORD", "DB_NAME", "DB_SSLMODE", "AWS_PROFILE", "AWS_SESSION_TOKEN"} {
		_ = os.Unsetenv(key)
	}
	for key, value := range map[string]string{
		"DATABASE_DSN":                dsn,
		"AWS_ACCESS_KEY_ID":           "e2e",
		"AWS_SECRET_ACCESS_KEY":       "e2e",
		"AWS_CONFIG_FILE":             os.DevNull,
		"AWS_SHARED_CREDENTIALS_FILE": os.DevNull,
	} {
		_ = os.Setenv(key, value)
	}

	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		return fail(fmt.Errorf("generate signing key: %w", err))
	}
	jwks := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(jose.JSONWebKeySet{Keys: []jose.JSONWebKey{{
			Key: &key.PublicKey, KeyID: e2eKeyID, Algorithm: string(jose.PS256), Use: "sig",
		}}})
	}))
	closers = append(closers, jwks.Close)

	storeDir, err := os.MkdirTemp("", "mentorship-e2e-nats-")
	if err != nil {
		return fail(err)
	}
	closers = append(closers, func() { _ = os.RemoveAll(storeDir) })
	ns, err := natsserver.NewServer(&natsserver.Options{Host: "127.0.0.1", Port: -1, JetStream: true, StoreDir: storeDir, NoLog: true, NoSigs: true})
	if err != nil {
		return fail(fmt.Errorf("start NATS: %w", err))
	}
	ns.Start()
	closers = append(closers, ns.Shutdown)
	if !ns.ReadyForConnections(10 * time.Second) {
		return fail(fmt.Errorf("NATS did not become ready"))
	}
	nc, err := nats.Connect(ns.ClientURL())
	if err != nil {
		return fail(fmt.Errorf("connect NATS: %w", err))
	}
	closers = append(closers, nc.Close)
	js, err := jetstream.New(nc)
	if err != nil {
		return fail(err)
	}
	for name, subject := range map[string]string{"E2E_FGA": "lfx.fga-sync.>", "E2E_INDEX": "lfx.index.>"} {
		if _, err := js.CreateStream(ctx, jetstream.StreamConfig{Name: name, Subjects: []string{subject}, Storage: jetstream.MemoryStorage}); err != nil {
			return fail(fmt.Errorf("create stream %s: %w", name, err))
		}
	}
	fk, err := startFakes(nc)
	if err != nil {
		return fail(err)
	}

	s3Backend := s3mem.New()
	s3 := httptest.NewServer(gofakes3.New(s3Backend).Server())
	closers = append(closers, s3.Close)
	crowdfunding := httptest.NewServer(fk.crowdfundingHandler())
	closers = append(closers, crowdfunding.Close)

	cfg := &Config{
		Server:   ServerConfig{ReadTimeout: 15 * time.Second, WriteTimeout: 30 * time.Second, IdleTimeout: time.Minute, ShutdownTimeout: 10 * time.Second},
		Database: DatabaseConfig{MaxConns: 10, MinConns: 1, ConnMaxLifetime: 30 * time.Minute},
		JWT:      JWTConfig{HeimdallJWKSURL: jwks.URL, HeimdallAudience: e2eAudience, HeimdallIssuer: e2eIssuer},
		FGA: FGAConfig{
			NATSURL: ns.ClientURL(), RelayBatch: 50, RelayInterval: 50 * time.Millisecond,
			RelayRetryDelay: time.Second, RelayMaxAttempts: 3,
		},
		Indexer:      IndexerConfig{ServiceToken: "e2e-indexer-token", RetryDelay: time.Second, MaxAttempts: 3},
		Email:        EmailConfig{PublicSiteURL: e2eSiteURL, SelfServeURL: e2eSelfServeURL, HRInbox: e2eHRInbox},
		Crowdfunding: CrowdfundingConfig{BaseURL: crowdfunding.URL, Timeout: 5 * time.Second},
		Storage: StorageConfig{
			Region:      "us-east-1",
			Logos:       BucketConfig{Bucket: e2eLogosBucket, EndpointURL: s3.URL, CreateMissingBucket: true, CDNURLPrefix: e2eCDNPrefix},
			Attachments: BucketConfig{Bucket: e2eFilesBucket, EndpointURL: s3.URL, CreateMissingBucket: true},
		},
		Local: LocalConfig{InviteSecret: e2eInviteSecret},
	}
	logLevel := slog.LevelError
	if os.Getenv("E2E_VERBOSE") != "" {
		logLevel = slog.LevelDebug
	}
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: logLevel}))
	slog.SetDefault(logger)

	srv, err := NewServer(ctx, cfg, logger)
	if err != nil {
		return fail(fmt.Errorf("new server: %w", err))
	}
	closers = append(closers, func() {
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		_ = srv.Shutdown(shutdownCtx)
	})
	api := httptest.NewServer(srv.router)
	closers = append(closers, api.Close)

	return &environment{baseURL: api.URL, pool: srv.pool, key: key, js: js, fakes: fk, s3: s3Backend}, cleanup, nil
}

// reset empties every table and every fake, so each test starts from a blank platform.
func reset(t *testing.T) {
	t.Helper()
	ctx := context.Background()
	if _, err := e2e.pool.Exec(ctx, `TRUNCATE users, user_profiles, programs, program_skills, program_funding_stats,
		program_terms, program_members, applications, tasks, quarantined_tasks, mentorship_approver_team_members,
		fga_outbox, index_outbox, object_deletions RESTART IDENTITY CASCADE`); err != nil {
		t.Fatalf("reset tables: %v", err)
	}
	for _, name := range []string{"E2E_FGA", "E2E_INDEX"} {
		stream, err := e2e.js.Stream(ctx, name)
		if err != nil {
			t.Fatalf("stream %s: %v", name, err)
		}
		if err := stream.Purge(ctx); err != nil {
			t.Fatalf("purge %s: %v", name, err)
		}
	}
	for _, bucket := range []string{e2eLogosBucket, e2eFilesBucket} {
		list, err := e2e.s3.ListBucket(bucket, nil, gofakes3.ListBucketPage{})
		if err != nil {
			t.Fatalf("list %s: %v", bucket, err)
		}
		for _, obj := range list.Contents {
			if _, err := e2e.s3.DeleteObject(bucket, obj.Key); err != nil {
				t.Fatalf("empty %s: %v", bucket, err)
			}
		}
	}
	e2e.fakes.reset()
	t.Cleanup(func() { awaitOutboxesDrained(t) })
}

// awaitOutboxesDrained waits for the relays to publish every outbox row the test wrote,
// and fails the test if any row was dead-lettered.
func awaitOutboxesDrained(t *testing.T) {
	t.Helper()
	const undrained = `SELECT 'fga' AS outbox, object_type, object_uid::text, state, generation, claimed_generation, attempts, COALESCE(last_error, '')
		FROM fga_outbox WHERE state <> 'dead_letter'
		UNION ALL
		SELECT 'index', object_type, object_uid::text, state, generation, claimed_generation, attempts, COALESCE(last_error, '')
		FROM index_outbox WHERE state IN ('pending', 'in_flight')`
	deadline := time.Now().Add(10 * time.Second)
	for dbCount(t, `SELECT count(*) FROM (`+undrained+`) u`) > 0 {
		if time.Now().After(deadline) {
			rows, err := e2e.pool.Query(context.Background(), undrained)
			if err == nil {
				for rows.Next() {
					values, _ := rows.Values()
					t.Logf("undrained outbox row: %v", values)
				}
				rows.Close()
			}
			t.Fatalf("timed out waiting for the FGA and index outboxes to drain")
		}
		time.Sleep(25 * time.Millisecond)
	}
	if n := dbCount(t, `SELECT (SELECT count(*) FROM fga_outbox WHERE state = 'dead_letter')
		+ (SELECT count(*) FROM index_outbox WHERE state = 'dead_letter')`); n != 0 {
		t.Errorf("%d outbox rows were dead-lettered", n)
	}
}

// actor is a caller of the API. An actor with an empty token is anonymous.
type actor struct {
	t     *testing.T
	token string
	// ID is the local user ID, set once the actor is provisioned.
	ID    string
	LFID  string
	Email string
}

type tokenClaims struct {
	Issuer    string `json:"iss"`
	Audience  string `json:"aud"`
	ExpiresAt int64  `json:"exp"`
	IssuedAt  int64  `json:"iat"`
	Principal string `json:"principal"`
	Email     string `json:"email,omitempty"`
	Scope     string `json:"scope,omitempty"`
}

// mintToken signs a Heimdall-shaped token with the harness key.
func mintToken(t *testing.T, claims tokenClaims) string {
	t.Helper()
	signer, err := jose.NewSigner(jose.SigningKey{Algorithm: jose.PS256, Key: e2e.key},
		(&jose.SignerOptions{}).WithHeader(jose.HeaderKey("kid"), e2eKeyID))
	if err != nil {
		t.Fatalf("new signer: %v", err)
	}
	payload, err := json.Marshal(claims)
	if err != nil {
		t.Fatalf("marshal claims: %v", err)
	}
	signed, err := signer.Sign(payload)
	if err != nil {
		t.Fatalf("sign token: %v", err)
	}
	token, err := signed.CompactSerialize()
	if err != nil {
		t.Fatalf("serialize token: %v", err)
	}
	return token
}

func validClaims(lfid, email, scope string) tokenClaims {
	now := time.Now()
	return tokenClaims{Issuer: e2eIssuer, Audience: e2eAudience, ExpiresAt: now.Add(time.Hour).Unix(), IssuedAt: now.Unix(), Principal: lfid, Email: email, Scope: scope}
}

// anonymous returns a caller with no token.
func anonymous(t *testing.T) *actor {
	return &actor{t: t}
}

// unprovisioned returns a signed-in caller that has no local user yet.
func unprovisioned(t *testing.T, lfid string, scopes ...string) *actor {
	email := lfid + "@e2e.example.org"
	return &actor{t: t, LFID: lfid, Email: email, token: mintToken(t, validClaims(lfid, email, strings.Join(append([]string{"access:me"}, scopes...), " ")))}
}

// signIn provisions the local user through PUT /me, as Self Serve does on first visit.
func signIn(t *testing.T, lfid string, scopes ...string) *actor {
	t.Helper()
	a := unprovisioned(t, lfid, scopes...)
	var user struct {
		ID string `json:"id"`
	}
	a.mustJSON(http.MethodPut, "/me", map[string]any{"name": "User " + lfid, "given_name": "User", "family_name": lfid}, http.StatusOK, &user)
	a.ID = user.ID
	return a
}

type response struct {
	t      *testing.T
	Status int
	Header http.Header
	Body   []byte
}

// decode unmarshals the body into v.
func (r *response) decode(v any) {
	r.t.Helper()
	if err := json.Unmarshal(r.Body, v); err != nil {
		r.t.Fatalf("decode %s: %v", r.Body, err)
	}
}

// expect fails the test unless the response has the given status.
func (r *response) expect(status int) *response {
	r.t.Helper()
	if r.Status != status {
		r.t.Fatalf("got status %d; want %d; body: %s", r.Status, status, r.Body)
	}
	return r
}

// errorMessage returns the "error" field of a JSON error body.
func (r *response) errorMessage() string {
	r.t.Helper()
	var body struct {
		Error string `json:"error"`
	}
	r.decode(&body)
	return body.Error
}

// call sends a request with a JSON body (nil for none) to path under the API prefix.
func (a *actor) call(method, path string, body any) *response {
	a.t.Helper()
	var reader io.Reader
	if body != nil {
		data, err := json.Marshal(body)
		if err != nil {
			a.t.Fatalf("marshal body: %v", err)
		}
		reader = bytes.NewReader(data)
	}
	return a.send(method, path, reader, "application/json", nil)
}

// send issues a raw request; headers are added after the default ones.
func (a *actor) send(method, path string, body io.Reader, contentType string, headers map[string]string) *response {
	a.t.Helper()
	req, err := http.NewRequest(method, e2e.baseURL+apiPrefix+path, body)
	if err != nil {
		a.t.Fatalf("new request: %v", err)
	}
	if body != nil && contentType != "" {
		req.Header.Set("Content-Type", contentType)
	}
	if a.token != "" {
		req.Header.Set("Authorization", "Bearer "+a.token)
	}
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		a.t.Fatalf("%s %s: %v", method, path, err)
	}
	defer func() { _ = res.Body.Close() }()
	data, err := io.ReadAll(res.Body)
	if err != nil {
		a.t.Fatalf("read body: %v", err)
	}
	return &response{t: a.t, Status: res.StatusCode, Header: res.Header, Body: data}
}

// mustJSON sends body, requires status, and decodes the response into out when out is non-nil.
func (a *actor) mustJSON(method, path string, body any, status int, out any) *response {
	a.t.Helper()
	res := a.call(method, path, body).expect(status)
	if out != nil {
		res.decode(out)
	}
	return res
}

// eventually polls check until it returns true or the timeout passes.
func eventually(t *testing.T, timeout time.Duration, what string, check func() bool) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if check() {
			return
		}
		time.Sleep(25 * time.Millisecond)
	}
	t.Fatalf("timed out after %s waiting for %s", timeout, what)
}

// streamMessages returns the payloads published to subject on the named JetStream stream.
func streamMessages(t *testing.T, streamName, subject string) [][]byte {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	stream, err := e2e.js.Stream(ctx, streamName)
	if err != nil {
		t.Fatalf("stream %s: %v", streamName, err)
	}
	info, err := stream.Info(ctx)
	if err != nil {
		t.Fatalf("stream info: %v", err)
	}
	var out [][]byte
	for seq := info.State.FirstSeq; seq <= info.State.LastSeq && info.State.Msgs > 0; seq++ {
		msg, err := stream.GetMsg(ctx, seq)
		if err != nil {
			continue
		}
		if msg.Subject == subject {
			out = append(out, msg.Data)
		}
	}
	return out
}

// dbCount returns the result of a count query.
func dbCount(t *testing.T, query string, args ...any) int {
	t.Helper()
	var n int
	if err := e2e.pool.QueryRow(context.Background(), query, args...).Scan(&n); err != nil {
		t.Fatalf("count query: %v", err)
	}
	return n
}

// dbExec runs a statement used to arrange state the API cannot reach, such as past dates.
func dbExec(t *testing.T, query string, args ...any) {
	t.Helper()
	if _, err := e2e.pool.Exec(context.Background(), query, args...); err != nil {
		t.Fatalf("exec: %v", err)
	}
}
