package usecase

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/furkanmeclis/clickfix-app-go/backend/internal/database/db"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/categories/classify"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/categories/model"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/supportops"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/ai"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/orgctx"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/rbac"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
)

var timeNow = time.Now

// Subject is one record that carries a category.
type Subject struct {
	ID           int64
	UUID         uuid.UUID
	DepartmentID int64
	Text         string
	Locale       string
}

// SubjectSource loads a record kind. With actor > 0 it enforces that the
// caller may see the record (ErrNotFound otherwise); actor 0 is the system
// (worker) path.
type SubjectSource interface {
	Load(ctx context.Context, org, actor int64, id uuid.UUID) (Subject, error)
}

// RegisterSubject plugs a record kind in (CF-216 knowledge units, CF-217
// archive records).
func (s *Service) RegisterSubject(kind string, src SubjectSource) {
	if classify.ValidSubjectType(kind) && src != nil {
		s.subjects[kind] = src
	}
}

func (s *Service) load(ctx context.Context, org, actor int64, kind string, id uuid.UUID) (Subject, error) {
	src, ok := s.subjects[kind]
	if !ok {
		return Subject{}, ErrUnknownSubject
	}
	sub, err := src.Load(ctx, org, actor, id)
	if errors.Is(err, pgx.ErrNoRows) || errors.Is(err, supportops.ErrDenied) {
		return Subject{}, ErrNotFound
	}
	return sub, err
}

type conversationSource struct{ q *db.Queries }

func (c conversationSource) Load(ctx context.Context, org, actor int64, id uuid.UUID) (Subject, error) {
	if actor > 0 {
		conv, text, err := supportops.Transcript(ctx, c.q, org, actor, id)
		if errors.Is(err, supportops.ErrInvalid) {
			err, text = nil, ""
		}
		if err != nil {
			return Subject{}, err
		}
		return Subject{ID: conv.ID, UUID: conv.Uuid, Text: text}, nil
	}
	conv, err := c.q.GetConversationByUUID(ctx, db.GetConversationByUUIDParams{OrganizationID: org, Uuid: id})
	if err != nil {
		return Subject{}, err
	}
	return c.system(ctx, org, conv)
}

func (c conversationSource) system(ctx context.Context, org int64, conv db.Conversation) (Subject, error) {
	rows, err := c.q.OpsTranscript(ctx, db.OpsTranscriptParams{OrganizationID: org, ConversationID: conv.ID})
	if err != nil {
		return Subject{}, err
	}
	var b strings.Builder
	for i := len(rows) - 1; i >= 0; i-- {
		line := rows[i].SenderType + ": " + rows[i].Content + "\n"
		if b.Len()+len(line) > 16000 {
			break
		}
		b.WriteString(line)
	}
	return Subject{ID: conv.ID, UUID: conv.Uuid, Text: b.String()}, nil
}

type suggestionSource struct{ q *db.Queries }

func (c suggestionSource) Load(ctx context.Context, org, actor int64, id uuid.UUID) (Subject, error) {
	if actor > 0 {
		if scope, ok := orgctx.From(ctx); !ok || !scope.Has(rbac.PermTenantSettingsRead) {
			return Subject{}, ErrNotFound
		}
	}
	row, err := c.q.CategorySuggestionSubject(ctx, db.CategorySuggestionSubjectParams{OrganizationID: org, Uuid: id})
	if err != nil {
		return Subject{}, err
	}
	return Subject{ID: row.ID, UUID: row.Uuid, Text: strings.TrimSpace(row.Topic + "\n" + row.Title + "\n" + row.Body)}, nil
}

func (s *Service) candidates(ctx context.Context, org, department int64) ([]classify.Candidate, error) {
	rows, err := s.q.CategoryClassificationCandidates(ctx, db.CategoryClassificationCandidatesParams{OrganizationID: org, DepartmentID: optID(department)})
	if err != nil {
		return nil, err
	}
	out := make([]classify.Candidate, 0, len(rows))
	for _, r := range rows {
		out = append(out, classify.Candidate{ID: r.ID, ParentID: r.ParentID.Int64, DepartmentID: r.DepartmentID.Int64, Name: r.Name, Slug: r.Slug, Description: r.Description})
	}
	return classify.Paths(out), nil
}

func optID(v int64) pgtype.Int8 { return pgtype.Int8{Int64: v, Valid: v > 0} }

// Assignment returns the current category of a record (nil when none yet).
func (s *Service) Assignment(ctx context.Context, org, actor int64, kind string, id uuid.UUID) (*model.Assignment, error) {
	sub, err := s.load(ctx, org, actor, kind, id)
	if err != nil {
		return nil, err
	}
	return s.assignment(ctx, org, kind, sub.ID)
}

func (s *Service) assignment(ctx context.Context, org int64, kind string, subjectID int64) (*model.Assignment, error) {
	row, err := s.q.GetCategoryAssignment(ctx, db.GetCategoryAssignmentParams{OrganizationID: org, SubjectType: kind, SubjectID: subjectID})
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	a := toAssignment(row.SubjectType, row.SubjectUuid, row.CategoryUuid, row.CategoryName, row.CategorySlug, row.Confidence, row.Source, row.Classifier, row.NeedsReview, row.UpdatedAt)
	return &a, nil
}

func toAssignment(kind string, subject uuid.UUID, cat pgtype.UUID, name, slug pgtype.Text, conf float32, source, classifier string, review bool, updated pgtype.Timestamptz) model.Assignment {
	a := model.Assignment{SubjectType: kind, SubjectUUID: subject, Confidence: float64(conf), Source: source, Classifier: classifier, NeedsReview: review, UpdatedAt: updated.Time}
	if cat.Valid {
		a.Category = &model.CategoryRef{UUID: uuid.UUID(cat.Bytes), Name: name.String, Slug: slug.String}
	}
	return a
}

// ClassifyRecord (re)classifies a record through the classifier. A human
// decision is never overwritten; force re-runs a machine decision.
func (s *Service) ClassifyRecord(ctx context.Context, org, actor int64, kind string, id uuid.UUID, force bool) (*model.Assignment, error) {
	sub, err := s.load(ctx, org, actor, kind, id)
	if err != nil {
		return nil, err
	}
	return s.classifySubject(ctx, org, actor, kind, sub, force)
}

// ClassifySubject is the entry point for record kinds that load their own
// data (CF-217 archive records): it classifies the text and stores the
// decision on the record.
func (s *Service) ClassifySubject(ctx context.Context, org int64, kind string, sub Subject, force bool) (*model.Assignment, error) {
	if !classify.ValidSubjectType(kind) || sub.ID <= 0 || sub.UUID == uuid.Nil {
		return nil, ErrUnknownSubject
	}
	return s.classifySubject(ctx, org, 0, kind, sub, force)
}

func (s *Service) classifySubject(ctx context.Context, org, actor int64, kind string, sub Subject, force bool) (*model.Assignment, error) {
	if strings.TrimSpace(sub.Text) == "" {
		return nil, ErrNoText
	}
	if existing, err := s.assignment(ctx, org, kind, sub.ID); err != nil {
		return nil, err
	} else if existing != nil && (existing.Source == classify.SourceHuman || !force) {
		return existing, nil
	}
	cands, err := s.candidates(ctx, org, sub.DepartmentID)
	if err != nil {
		return nil, err
	}
	if len(cands) == 0 {
		return nil, ErrNoCategories
	}
	ctx = orgctx.WithOrganizationID(ctx, org)
	res, err := s.classifier.Classify(ctx, classify.Input{OrganizationID: org, DepartmentID: sub.DepartmentID, SubjectType: kind, Text: sub.Text, Locale: sub.Locale, ActorID: actor}, cands)
	if err != nil {
		return nil, err
	}
	source, classifier := res.Source, res.Classifier
	if source == "" { // no stage answered: an agent decides
		source, classifier = classify.SourceAI, "none"
	}
	n, err := s.q.InsertCategoryAssignmentIfAbsent(ctx, db.InsertCategoryAssignmentIfAbsentParams{
		OrganizationID: org, SubjectType: kind, SubjectID: sub.ID, SubjectUuid: sub.UUID, DepartmentID: optID(sub.DepartmentID),
		CategoryID: optID(res.CategoryID), Confidence: float32(res.Confidence), Source: source, Classifier: classifier, NeedsReview: res.NeedsReview,
	})
	if err != nil {
		return nil, err
	}
	if n == 0 {
		if _, err := s.q.ReplaceAutoCategoryAssignment(ctx, db.ReplaceAutoCategoryAssignmentParams{
			OrganizationID: org, SubjectType: kind, SubjectID: sub.ID, DepartmentID: optID(sub.DepartmentID),
			CategoryID: optID(res.CategoryID), Confidence: float32(res.Confidence), Source: source, Classifier: classifier, NeedsReview: res.NeedsReview,
		}); err != nil {
			return nil, err
		}
	}
	return s.assignment(ctx, org, kind, sub.ID)
}

// Label records an agent's decision: a confirmation when the category stays
// the same, a correction otherwise. The record takes the category with
// source "human"; the masked text and its vector go to the label store.
func (s *Service) Label(ctx context.Context, org, actor int64, kind string, id uuid.UUID, category uuid.UUID) (*model.Assignment, error) {
	if actor <= 0 {
		return nil, ErrNotFound
	}
	sub, err := s.load(ctx, org, actor, kind, id)
	if err != nil {
		return nil, err
	}
	cat, err := s.q.GetCategoryByUUID(ctx, db.GetCategoryByUUIDParams{OrganizationID: org, Uuid: category})
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, invalid("category_uuid", "not_found", "category not found")
	}
	if err != nil {
		return nil, err
	}
	if cat.ArchivedAt.Valid {
		return nil, invalid("category_uuid", "archived", "category is archived")
	}
	if cat.DepartmentID.Valid && sub.DepartmentID > 0 && cat.DepartmentID.Int64 != sub.DepartmentID {
		return nil, invalid("category_uuid", "department", "category belongs to another department")
	}
	text := classify.Truncate(ai.NewMasker().Mask(sub.Text), 24000)
	var vector []float32
	model := ""
	if s.embedder != nil && text != "" {
		// Best effort: a label without a vector is still training data and
		// is vectorized again on export (CF-219) or re-indexing (CF-214).
		if out, err := s.embedder.Embed(orgctx.WithOrganizationID(ctx, org), []string{classify.Truncate(text, 6000)}); err == nil && len(out.Vectors) == 1 && len(out.Vectors[0]) > 0 {
			vector, model = out.Vectors[0], s.embedder.Model()
		}
	}
	err = s.withTx(ctx, func(q *db.Queries) error {
		prev, err := q.GetCategoryAssignment(ctx, db.GetCategoryAssignmentParams{OrganizationID: org, SubjectType: kind, SubjectID: sub.ID})
		hasPrev := err == nil
		if err != nil && !errors.Is(err, pgx.ErrNoRows) {
			return err
		}
		action := "correct"
		if hasPrev && prev.CategoryID.Valid && prev.CategoryID.Int64 == cat.ID {
			action = "confirm"
		}
		if text != "" {
			p := db.InsertCategoryLabelParams{
				OrganizationID: org, DepartmentID: optID(sub.DepartmentID), SubjectType: kind, SubjectUuid: sub.UUID, Action: action,
				Content: text, Embedding: vector, EmbeddingModel: model, CategoryID: cat.ID, LabeledBy: actor,
			}
			if hasPrev {
				p.PreviousCategoryID = prev.CategoryID
				p.PreviousSource = pgtype.Text{String: prev.Source, Valid: true}
				p.PreviousConfidence = pgtype.Float4{Float32: prev.Confidence, Valid: true}
			}
			if _, err := q.InsertCategoryLabel(ctx, p); err != nil {
				return err
			}
		}
		_, err = q.UpsertCategoryAssignment(ctx, db.UpsertCategoryAssignmentParams{
			OrganizationID: org, SubjectType: kind, SubjectID: sub.ID, SubjectUuid: sub.UUID, DepartmentID: optID(sub.DepartmentID),
			CategoryID: pgtype.Int8{Int64: cat.ID, Valid: true}, Confidence: 1, Source: classify.SourceHuman, Classifier: "human",
			NeedsReview: false, AssignedBy: pgtype.Int8{Int64: actor, Valid: true},
		})
		return err
	})
	if err != nil {
		return nil, err
	}
	return s.assignment(ctx, org, kind, sub.ID)
}

// Clear removes a record's category (it is classified again on demand).
func (s *Service) Clear(ctx context.Context, org, actor int64, kind string, id uuid.UUID) error {
	sub, err := s.load(ctx, org, actor, kind, id)
	if err != nil {
		return err
	}
	_, err = s.q.DeleteCategoryAssignment(ctx, db.DeleteCategoryAssignmentParams{OrganizationID: org, SubjectType: kind, SubjectID: sub.ID})
	return err
}

// ReviewQueue lists the records an agent should confirm or correct.
func (s *Service) ReviewQueue(ctx context.Context, org int64, f model.ReviewFilter) ([]model.Assignment, int64, error) {
	if f.SubjectType != "" && !classify.ValidSubjectType(f.SubjectType) {
		return nil, 0, invalid("subject_type", "invalid", "unknown record kind")
	}
	rows, err := s.q.ListCategoryReviewQueue(ctx, db.ListCategoryReviewQueueParams{OrganizationID: org, SubjectType: f.SubjectType, DepartmentID: optID(f.DepartmentID), LimitCount: f.Limit, OffsetCount: f.Offset})
	if err != nil {
		return nil, 0, err
	}
	total, err := s.q.CountCategoryReviewQueue(ctx, db.CountCategoryReviewQueueParams{OrganizationID: org, SubjectType: f.SubjectType, DepartmentID: optID(f.DepartmentID)})
	if err != nil {
		return nil, 0, err
	}
	out := make([]model.Assignment, 0, len(rows))
	for _, r := range rows {
		out = append(out, toAssignment(r.SubjectType, r.SubjectUuid, r.CategoryUuid, r.CategoryName, r.CategorySlug, r.Confidence, r.Source, r.Classifier, r.NeedsReview, r.UpdatedAt))
	}
	return out, total, nil
}

// ExportLabels streams the training set in keyset pages (CF-219).
func (s *Service) ExportLabels(ctx context.Context, org int64, since time.Time, emit func(model.LabelExport) error) error {
	after := int64(0)
	sinceTS := pgtype.Timestamptz{Time: since, Valid: !since.IsZero()}
	depts := map[int64]uuid.UUID{}
	for {
		rows, err := s.q.ExportCategoryLabels(ctx, db.ExportCategoryLabelsParams{OrganizationID: org, AfterID: after, Since: sinceTS, LimitCount: 500})
		if err != nil {
			return err
		}
		for _, r := range rows {
			line := model.LabelExport{
				UUID: r.Uuid, CreatedAt: r.CreatedAt.Time, SubjectType: r.SubjectType, Action: r.Action, Text: r.Content,
				EmbeddingModel: r.EmbeddingModel, Embedding: r.Embedding, LabeledBy: r.LabeledByUuid,
				Category: model.CategoryRef{UUID: r.CategoryUuid, Name: r.CategoryName, Slug: r.CategorySlug},
			}
			if line.Embedding == nil {
				line.Embedding = []float32{}
			}
			line.DepartmentUUID = s.deptUUID(ctx, org, r.DepartmentID, depts)
			if r.PreviousCategoryUuid.Valid {
				line.PreviousCategory = &model.CategoryRef{UUID: uuid.UUID(r.PreviousCategoryUuid.Bytes), Name: r.PreviousCategoryName.String, Slug: r.PreviousCategorySlug.String}
			}
			if r.PreviousSource.Valid {
				src := r.PreviousSource.String
				line.PreviousSource = &src
			}
			if r.PreviousConfidence.Valid {
				c := float64(r.PreviousConfidence.Float32)
				line.PreviousConf = &c
			}
			if err := emit(line); err != nil {
				return err
			}
			after = r.ID
		}
		if len(rows) < 500 {
			return nil
		}
	}
}

// vectorStore is the classify.VectorStore over the category tables.
type vectorStore struct{ q *db.Queries }

func (v *vectorStore) MissingExamples(ctx context.Context, org, department int64, model string) ([]classify.Example, error) {
	rows, err := v.q.CategoryExamplesMissing(ctx, db.CategoryExamplesMissingParams{OrganizationID: org, DepartmentID: optID(department), EmbeddingModel: model})
	if err != nil {
		return nil, err
	}
	out := make([]classify.Example, 0, len(rows))
	for _, r := range rows {
		out = append(out, classify.Example{CategoryID: r.CategoryID, Content: r.Content})
	}
	return out, nil
}

func (v *vectorStore) SaveExamples(ctx context.Context, org int64, model string, examples []classify.Example, vectors [][]float32) error {
	for i, e := range examples {
		if len(vectors[i]) == 0 {
			continue
		}
		if err := v.q.InsertCategoryExample(ctx, db.InsertCategoryExampleParams{OrganizationID: org, CategoryID: e.CategoryID, Content: e.Content, Embedding: vectors[i], EmbeddingModel: model}); err != nil {
			return err
		}
	}
	return nil
}

func (v *vectorStore) Similar(ctx context.Context, org, department int64, model string, vector []float32) ([]classify.Score, error) {
	rows, err := v.q.CategorySimilarity(ctx, db.CategorySimilarityParams{OrganizationID: org, DepartmentID: optID(department), EmbeddingModel: model, Embedding: vector})
	if err != nil {
		return nil, err
	}
	out := make([]classify.Score, 0, len(rows))
	for _, r := range rows {
		out = append(out, classify.Score{CategoryID: r.CategoryID, Score: r.Score})
	}
	return out, nil
}
