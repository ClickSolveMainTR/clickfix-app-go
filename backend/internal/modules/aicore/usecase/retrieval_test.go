package usecase

import (
	"context"
	"testing"
	"time"

	kbmodel "github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/knowledge/model"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/vectorstore"
)

type blockingRetrieval struct{ started, release chan struct{} }

func (r *blockingRetrieval) SearchScoped(ctx context.Context, _ vectorstore.Scope, _ string, _ string, publicOnly bool) ([]kbmodel.Citation, error) {
	if !publicOnly {
		panic("prefetch must be public only")
	}
	close(r.started)
	select {
	case <-r.release:
		return []kbmodel.Citation{}, nil
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

func TestPrefetchReplyCanOverlapTriageAndJoinCancellation(t *testing.T) {
	svc, _, org, _ := aiFixture(t)
	if _, err := svc.Repo.Billing.Pool.Exec(context.Background(), "UPDATE ai_policies SET concurrency=3 WHERE organization_id=$1", org); err != nil {
		t.Fatal(err)
	}
	r := &blockingRetrieval{started: make(chan struct{}), release: make(chan struct{})}
	svc.RAG = r
	ctx, cancel := context.WithCancel(context.Background())
	result := svc.PrefetchReply(ctx, vectorstore.Organization(org), "Siparişim gelmedi", "tr")
	if result == nil {
		t.Fatal("prefetch not started")
	}
	select {
	case <-r.started:
	case <-time.After(time.Second):
		t.Fatal("retrieval did not start independently")
	}
	cancel()
	select {
	case out := <-result:
		if out.Err == nil {
			t.Fatal("cancellation lost")
		}
	case <-time.After(time.Second):
		t.Fatal("prefetch leaked")
	}
	if _, open := <-result; open {
		t.Fatal("result channel not closed")
	}
}

func TestPrefetchReplyRespectsSingleCallPolicy(t *testing.T) {
	svc, _, org, _ := aiFixture(t)
	if _, err := svc.Repo.Billing.Pool.Exec(context.Background(), "UPDATE ai_policies SET concurrency=1 WHERE organization_id=$1", org); err != nil {
		t.Fatal(err)
	}
	svc.RAG = &blockingRetrieval{}
	if result := svc.PrefetchReply(context.Background(), vectorstore.Organization(org), "help", "tr"); result != nil {
		t.Fatal("concurrency=1 must keep sequential retrieval")
	}
}
