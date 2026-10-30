package inbox_test

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"io"
	"log"
	"net"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/emersion/go-imap/backend/memory"
	"github.com/emersion/go-imap/server"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/database/db"
	channelsrepository "github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/channels/repository"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/channels"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/channels/email"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/crypto"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
)

func TestIMAPPollThreadDedupeAndHealth(t *testing.T) {
	e := setup(t)
	ctx := context.Background()
	q := db.New(e.pool)
	// A real TLS IMAP listener with an in-process memory mailbox; no provider I/O.
	be := memory.New()
	user, _ := be.Login(nil, "username", "password")
	mb, _ := user.GetMailbox("INBOX")
	box := mb.(*memory.Mailbox)
	raw := []byte("From: customer@example.test\r\nTo: support@company.test\r\nSubject: Re: Support\r\nMessage-ID: <reply@company.test>\r\nReferences: <root@company.test>\r\nIn-Reply-To: <root@company.test>\r\nContent-Type: text/plain\r\n\r\nHello from IMAP")
	box.Messages = []*memory.Message{{Uid: 1, Date: time.Now(), Size: uint32(len(raw)), Body: raw}, {Uid: 2, Date: time.Now(), Size: uint32(len(raw)), Body: raw}}
	cert := httptest.NewTLSServer(nil)
	trust := x509.NewCertPool()
	trust.AddCert(cert.Certificate())
	tc := cert.TLS.Clone()
	cert.Close()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	srv := server.New(be)
	srv.TLSConfig = tc
	srv.ErrorLog = log.New(io.Discard, "", 0)
	done := make(chan struct{})
	go func() { _ = srv.Serve(tls.NewListener(listener, tc)); close(done) }()
	t.Cleanup(func() { _ = srv.Close(); <-done })
	host, port, _ := net.SplitHostPort(listener.Addr().String())
	adapter := email.New(email.Config{IMAPTLSConfig: &tls.Config{RootCAs: trust, ServerName: host, MinVersion: tls.VersionTLS12}})
	e.ib.Registry.Register(adapter)
	id := uuid.New()
	creds := channels.Credentials{email.FieldProvider: email.ProviderIMAP, email.FieldIMAPHost: host, email.FieldIMAPPort: port, email.FieldIMAPUsername: "username", email.FieldIMAPPassword: "password"}
	secretBox, err := crypto.NewSecretBox("0123456789abcdef0123456789abcdef")
	if err != nil {
		t.Fatal(err)
	}
	vault, _ := channels.NewVault(secretBox)
	sealed, err := vault.Seal(e.org, id, creds)
	if err != nil {
		t.Fatal(err)
	}
	row, err := q.InsertChannelInstance(ctx, db.InsertChannelInstanceParams{Uuid: id, OrganizationID: e.org, ChannelType: channels.TypeEmail, Name: "Mailbox", Status: "active", CredentialsEnc: sealed, Settings: []byte("{}")})
	if err != nil {
		t.Fatal(err)
	}
	locker := &channelsrepository.IMAP{Pool: e.pool}
	if err := locker.WithLock(ctx, row.ID, func() error {
		return locker.WithLock(ctx, row.ID, func() error { t.Error("overlapping mailbox lock entered"); return nil })
	}); err != nil {
		t.Fatal(err)
	}
	inst, _, err := e.ib.Channels.Instance(ctx, e.org, row.ID)
	if err != nil {
		t.Fatal(err)
	}
	root := []byte("From: customer@example.test\r\nTo: support@company.test\r\nMessage-ID: <root@company.test>\r\nSubject: Support\r\n\r\nFirst question")
	result, err := adapter.ParseRaw(root, inst, "")
	if err != nil {
		t.Fatal(err)
	}
	if err := e.ib.Receive(ctx, inst, result); err != nil {
		t.Fatal(err)
	}
	rootRow, err := q.GetMessageByExternal(ctx, db.GetMessageByExternalParams{OrganizationID: e.org, ChannelInstanceID: pgtype.Int8{Int64: row.ID, Valid: true}, ExternalID: pgtype.Text{String: "<root@company.test>", Valid: true}})
	if err != nil {
		t.Fatal(err)
	}
	if err := e.ib.Channels.PollIMAP(ctx, e.org, row.ID, e.ib.Receive); err != nil {
		t.Fatal(err)
	}
	var count int
	var conv int64
	if err := e.pool.QueryRow(ctx, "SELECT count(*),min(conversation_id) FROM messages WHERE organization_id=$1 AND channel_instance_id=$2 AND external_id=$3", e.org, row.ID, "<reply@company.test>").Scan(&count, &conv); err != nil {
		t.Fatal(err)
	}
	if count != 1 || conv != rootRow.ConversationID {
		t.Fatalf("dedupe/thread count=%d conv=%d want=%d", count, conv, rootRow.ConversationID)
	}
	state, err := q.GetEmailIMAPState(ctx, db.GetEmailIMAPStateParams{OrganizationID: e.org, ChannelInstanceID: row.ID})
	if err != nil || state.LastUid != 2 {
		t.Fatalf("cursor %+v %v", state, err)
	}
	// Three failed logins, with the persisted backoff advanced by the test clock.
	creds[email.FieldIMAPPassword] = "private-password-do-not-expose"
	sealed, _ = vault.Seal(e.org, id, creds)
	if _, err := e.pool.Exec(ctx, "UPDATE channel_instances SET credentials_enc=$1 WHERE organization_id=$2 AND id=$3", sealed, e.org, row.ID); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 3; i++ {
		if _, err := e.pool.Exec(ctx, "UPDATE channel_email_imap_states SET next_attempt_at=NOW()-interval '1 second' WHERE organization_id=$1 AND channel_instance_id=$2", e.org, row.ID); err != nil {
			t.Fatal(err)
		}
		if err := e.ib.Channels.PollIMAP(ctx, e.org, row.ID, e.ib.Receive); err != nil {
			t.Fatal(err)
		}
	}
	view, err := e.ib.Channels.Get(ctx, e.org, id)
	if err != nil {
		t.Fatal(err)
	}
	if view.Health.State != "error" || !strings.Contains(view.Health.Detail, "kimlik doğrulaması") || strings.Contains(view.Health.Detail, creds[email.FieldIMAPPassword]) {
		t.Fatalf("health %+v", view.Health)
	}
	state, err = q.GetEmailIMAPState(ctx, db.GetEmailIMAPStateParams{OrganizationID: e.org, ChannelInstanceID: row.ID})
	if err != nil || state.Failures != 3 || !state.NextAttemptAt.Time.After(time.Now().Add(7*time.Minute)) {
		t.Fatal(fmt.Sprint(state, err))
	}
	// Wrong-organization payloads cannot open or poll the instance.
	if err := e.ib.Channels.PollIMAP(ctx, e.org+999999, row.ID, e.ib.Receive); err != nil {
		t.Fatal(err)
	}
}
