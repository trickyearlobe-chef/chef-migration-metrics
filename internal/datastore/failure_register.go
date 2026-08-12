// Copyright 2025 Chef Migration Metrics Authors
// SPDX-License-Identifier: Apache-2.0

package datastore

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"
)

// ---------------------------------------------------------------------------
// The failure register
//
// A person's verdict on whether a cookbook actually works on the target
// version, recorded with a reason. Behaviour is
// journeys/human-verdict.md.
// ---------------------------------------------------------------------------

// The two sides of a verdict. One says "this is broken and you missed it", the
// other "this is not actually broken whatever the scan says". Both are the
// same act — a person overruling a machine, with evidence.
const (
	VerdictBroken    = "broken"
	VerdictNotBroken = "not_broken"
)

// An entry's lifecycle. Resolution is recorded rather than deleted because the
// standup view needs the direction of travel; supersession is what a reversal
// leaves behind, because verdicts are never silently replaced.
const (
	FailureStatusOpen       = "open"
	FailureStatusResolved   = "resolved"
	FailureStatusSuperseded = "superseded"
)

// What a verdict's subject names. A repo where one has been collected — that
// is where a fix is made and re-released — and the cookbook itself where none
// has.
const (
	SubjectTypeGitRepo  = "git_repo"
	SubjectTypeCookbook = "cookbook"
)

// Who is on it. Either an owner, or a reference to work tracked in another
// system — CMM holds that reference and does not read the system behind it.
//
// There is deliberately no separate "user" kind. Everything person-shaped is
// an owner, and other identities — including one sourced from SAML — reach an
// owner through an alias; the signed-in CMM user resolves to an owner the same
// way, which is what makes "what's mine" filtering possible. A second kind for
// people would be a second identity space for the same thing, which is the
// conflation journeys/ownership-identity.md exists to prevent.
const (
	HolderTypeOwner  = "owner"
	HolderTypeTicket = "ticket"
)

// FailureRegisterEntry is one person's verdict about one git repo.
type FailureRegisterEntry struct {
	ID string `json:"id"`

	// The subject is normally the repo, because that is where a fix is made.
	// Where no repo has been collected the subject is the cookbook itself —
	// those are more likely to be unowned and untested, not less, so refusing
	// them would exclude the population the register exists to catch.
	SubjectName string `json:"subject_name"`
	SubjectType string `json:"subject_type"`

	// The label, because standup says "cookbook". Never a version.
	CookbookName string `json:"cookbook_name"`

	Verdict string `json:"verdict"`
	Reason  string `json:"reason"`

	// Evidence is unbounded text — a stacktrace, or the fleet observation that
	// contradicts the scan. It must never reach an index.
	Evidence string `json:"evidence,omitempty"`

	Diagnosis string `json:"diagnosis,omitempty"`
	Plan      string `json:"plan,omitempty"`

	// TargetDate is a calendar date, YYYY-MM-DD, and empty where none has
	// been given. Deliberately not a timestamp: the column is a DATE, and
	// serialising it as midnight UTC both breaks a date input that has to
	// parse it back and renders the previous day for anyone west of UTC.
	TargetDate string `json:"target_date,omitempty"`

	HolderType string `json:"holder_type,omitempty"`
	HolderRef  string `json:"holder_ref,omitempty"`

	Status    string    `json:"status"`
	RaisedBy  string    `json:"raised_by"`
	RaisedAt  time.Time `json:"raised_at"`
	UpdatedAt time.Time `json:"updated_at"`

	// RaisedOrigin says what made this entry — OriginScreen or
	// OriginCredential. RaisedBy says whose judgement it is; this says whether
	// they typed it or a tool holding their credential wrote it, which is the
	// difference between a finding somebody read and one produced under their
	// name that they may never have seen.
	//
	// Attached by the service from the session, never taken from the request:
	// a caller that could set this could sign anything as a person's own.
	RaisedOrigin string `json:"raised_origin"`

	// RaisedOriginName is the credential's name, when one made the entry.
	// Empty for a screen. The name rather than the id, because this is read by
	// somebody deciding how much weight to give the entry.
	RaisedOriginName string `json:"raised_origin_name,omitempty"`

	ResolvedBy     string     `json:"resolved_by,omitempty"`
	ResolvedAt     *time.Time `json:"resolved_at,omitempty"`
	ResolutionNote string     `json:"resolution_note,omitempty"`

	// SupersededBy is the reversal that replaced this verdict. The losing
	// verdict stays readable so the disagreement is visible.
	SupersededBy string `json:"superseded_by,omitempty"`
}

// RecordFailureVerdictParams records a new verdict about a repo.
type RecordFailureVerdictParams struct {
	SubjectName  string
	SubjectType  string
	CookbookName string
	Verdict      string
	Reason       string
	Evidence     string
	Diagnosis    string
	Plan         string
	TargetDate   *time.Time
	HolderType   string
	HolderRef    string
	RaisedBy     string

	// RaisedOrigin and RaisedOriginName come from the session, not the body.
	// Empty RaisedOrigin is stored as OriginScreen, which is what every entry
	// was before credentials existed.
	RaisedOrigin     string
	RaisedOriginName string
}

// ReviseFailureEntryParams updates what is known and planned about an open
// entry. Every field is a pointer: nil leaves the stored value alone, so a
// revision that only sets a target date cannot blank a plan somebody else
// wrote. The verdict and the reason are deliberately absent — a change of
// verdict is a new entry that supersedes this one.
type ReviseFailureEntryParams struct {
	Diagnosis  *string
	Plan       *string
	Evidence   *string
	TargetDate *time.Time
	HolderType *string
	HolderRef  *string
}

// FailureRegisterFilter bounds a read of the register.
type FailureRegisterFilter struct {
	Status      string // open / resolved / superseded; empty means every status
	Verdict     string // broken / not_broken; empty means both
	SubjectName string
	Limit       int
	Offset      int
}

// StandingVerdict is a human verdict as the readiness evaluator consumes it:
// the current, unsuperseded, unresolved opinion about one repo.
type StandingVerdict struct {
	SubjectName  string
	SubjectType  string
	CookbookName string
	Verdict      string
	Reason       string
	RaisedBy     string
	RaisedAt     time.Time
}

// FailureRegisterSummary is what the standup view needs beyond the list
// itself: how large the register is, which way it is moving, and how much of
// it nobody has been put on.
type FailureRegisterSummary struct {
	// Size, now.
	Open              int `json:"open"`
	OpenBroken        int `json:"open_broken"`
	OpenNotBroken     int `json:"open_not_broken"`
	OpenWithoutHolder int `json:"open_without_holder"`
	OpenOverdue       int `json:"open_overdue"`

	// Direction of travel. More raised than resolved is a growing register,
	// which is a different message from a shrinking one.
	WindowDays       int `json:"window_days"`
	RaisedInWindow   int `json:"raised_in_window"`
	ResolvedInWindow int `json:"resolved_in_window"`

	// The whole history, which is the only measure the product has of how
	// wrong the automated signals are: every 'broken' entry is a failure they
	// missed, every 'not_broken' entry a verdict they got wrong.
	TotalBroken    int `json:"total_broken"`
	TotalNotBroken int `json:"total_not_broken"`
	Resolved       int `json:"resolved"`
}

// failureEntryColumns is the read projection, in the order scanFailureEntry
// expects.
const failureEntryColumns = `
	id, subject_name, subject_type, cookbook_name, verdict, reason,
	COALESCE(evidence, ''), COALESCE(diagnosis, ''), COALESCE(plan, ''), target_date,
	COALESCE(holder_type, ''), COALESCE(holder_ref, ''),
	status, raised_by, raised_at, updated_at,
	COALESCE(resolved_by, ''), resolved_at, COALESCE(resolution_note, ''),
	superseded_by,
	raised_origin, raised_origin_name
`

// What made an entry. A person at a screen, or a tool holding a credential
// somebody made and named.
const (
	// OriginScreen is somebody typing it at the web interface. Also what every
	// entry recorded before credentials existed is stored as, correctly: there
	// was no other way in.
	OriginScreen = "screen"
	// OriginCredential is a tool writing through a credential. The credential's
	// name is stored beside it so a later reader can see which one.
	OriginCredential = "credential"
)

func scanFailureEntry(s interface{ Scan(...any) error }) (FailureRegisterEntry, error) {
	var e FailureRegisterEntry
	var targetDate, resolvedAt sql.NullTime
	var supersededBy sql.NullString

	err := s.Scan(
		&e.ID, &e.SubjectName, &e.SubjectType, &e.CookbookName, &e.Verdict, &e.Reason,
		&e.Evidence, &e.Diagnosis, &e.Plan, &targetDate,
		&e.HolderType, &e.HolderRef,
		&e.Status, &e.RaisedBy, &e.RaisedAt, &e.UpdatedAt,
		&e.ResolvedBy, &resolvedAt, &e.ResolutionNote,
		&supersededBy,
		&e.RaisedOrigin, &e.RaisedOriginName,
	)
	if err != nil {
		return FailureRegisterEntry{}, err
	}
	if targetDate.Valid {
		e.TargetDate = targetDate.Time.Format("2006-01-02")
	}
	if resolvedAt.Valid {
		r := resolvedAt.Time
		e.ResolvedAt = &r
	}
	if supersededBy.Valid {
		e.SupersededBy = supersededBy.String
	}
	return e, nil
}

// raisedOrigin defaults an unset origin to the screen.
//
// A caller that does not set it is one written before credentials existed, and
// every such caller is a screen. Defaulting rather than rejecting because the
// alternative — a write path that fails when an origin is missing — turns a
// field nobody sends into an outage.
func raisedOrigin(origin string) string {
	if origin == OriginCredential {
		return OriginCredential
	}
	return OriginScreen
}

// validateVerdict rejects anything that is not one of the two sides.
func validateVerdict(verdict string) error {
	switch verdict {
	case VerdictBroken, VerdictNotBroken:
		return nil
	default:
		return fmt.Errorf("datastore: %q is not a verdict this register holds", verdict)
	}
}

// validateSubjectType rejects a subject that is neither a repo nor a cookbook.
// An empty value means a repo, which is what every entry recorded before
// cookbook subjects existed was.
func validateSubjectType(subjectType string) (string, error) {
	switch subjectType {
	case "", SubjectTypeGitRepo:
		return SubjectTypeGitRepo, nil
	case SubjectTypeCookbook:
		return SubjectTypeCookbook, nil
	default:
		return "", fmt.Errorf("datastore: %q is not a kind of subject a verdict can be about", subjectType)
	}
}

// resolveHolder normalises a commitment holder to the pair that gets stored.
//
// A reference is what makes a commitment chaseable, so it decides: with no
// reference there is no holder, and whatever kind was left behind is dropped
// rather than rejected. That is what lets an entry be unassigned again —
// requiring the pair to be emptied together made "nobody is on it" a state
// reachable only at the moment an entry was raised, so the count of unowned
// failures could only ever fall.
//
// A reference with no kind is still an error: it cannot be looked up, and
// guessing which system it belongs to is how a ticket becomes a person.
func resolveHolder(holderType, holderRef string) (string, string, error) {
	holderType = strings.TrimSpace(holderType)
	holderRef = strings.TrimSpace(holderRef)

	if holderRef == "" {
		return "", "", nil
	}
	switch holderType {
	case HolderTypeOwner, HolderTypeTicket:
		return holderType, holderRef, nil
	case "":
		return "", "", errors.New("datastore: a commitment holder needs to say what kind of reference it is")
	default:
		return "", "", fmt.Errorf("datastore: %q is not a kind of commitment holder", holderType)
	}
}

// RecordFailureVerdict records a person's verdict about a repo.
//
// If the repo already carries a standing verdict, this one supersedes it: the
// previous entry moves to 'superseded' and points at this one. It is never
// overwritten — who said what, when and why is the point of the register, and
// the disagreement is what a later reader needs to judge either verdict.
func (db *DB) RecordFailureVerdict(ctx context.Context, p RecordFailureVerdictParams) (FailureRegisterEntry, error) {
	if strings.TrimSpace(p.SubjectName) == "" {
		return FailureRegisterEntry{}, errors.New("datastore: a verdict needs the git repo it is about")
	}
	if strings.TrimSpace(p.CookbookName) == "" {
		return FailureRegisterEntry{}, errors.New("datastore: a verdict needs the cookbook it is labelled with")
	}
	subjectType, serr := validateSubjectType(p.SubjectType)
	if serr != nil {
		return FailureRegisterEntry{}, serr
	}
	if err := validateVerdict(p.Verdict); err != nil {
		return FailureRegisterEntry{}, err
	}
	// A verdict with no reason is an opinion, and it will be overturned by the
	// next person who disagrees. Checked here as well as in the schema so the
	// caller gets a sentence rather than a constraint name.
	if strings.TrimSpace(p.Reason) == "" {
		return FailureRegisterEntry{}, errors.New("datastore: a verdict needs a reason")
	}
	holderType, holderRef, err := resolveHolder(p.HolderType, p.HolderRef)
	if err != nil {
		return FailureRegisterEntry{}, err
	}
	if strings.TrimSpace(p.RaisedBy) == "" {
		return FailureRegisterEntry{}, errors.New("datastore: a verdict needs to say who recorded it")
	}

	var entry FailureRegisterEntry
	err = db.Tx(ctx, func(tx *sql.Tx) error {
		// The previous standing verdict for this repo, if there is one. Locked
		// so two people recording at once cannot both leave an open entry.
		var previousID sql.NullString
		err := tx.QueryRowContext(ctx, `
			SELECT id FROM failure_register_entries
			WHERE subject_name = $1 AND status = 'open'
			FOR UPDATE
		`, p.SubjectName).Scan(&previousID)
		if err != nil && !errors.Is(err, sql.ErrNoRows) {
			return fmt.Errorf("reading the standing verdict for %q: %w", p.SubjectName, err)
		}

		if previousID.Valid {
			// Stand the previous verdict down first: the partial unique index
			// allows only one open entry per repo.
			if _, err := tx.ExecContext(ctx, `
				UPDATE failure_register_entries
				SET status = 'superseded', updated_at = now()
				WHERE id = $1
			`, previousID.String); err != nil {
				return fmt.Errorf("superseding the previous verdict: %w", err)
			}
		}

		row := tx.QueryRowContext(ctx, `
			INSERT INTO failure_register_entries
				(subject_name, subject_type, cookbook_name, verdict, reason, evidence,
				 diagnosis, plan, target_date, holder_type, holder_ref, raised_by,
				 raised_origin, raised_origin_name)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14)
			RETURNING `+failureEntryColumns,
			p.SubjectName, subjectType, p.CookbookName, p.Verdict, strings.TrimSpace(p.Reason),
			nullStringPtr(p.Evidence), nullStringPtr(p.Diagnosis), nullStringPtr(p.Plan),
			nullTimePtr(p.TargetDate),
			nullStringPtr(holderType), nullStringPtr(holderRef),
			p.RaisedBy,
			raisedOrigin(p.RaisedOrigin), p.RaisedOriginName,
		)
		entry, err = scanFailureEntry(row)
		if err != nil {
			return fmt.Errorf("recording the verdict: %w", err)
		}

		if previousID.Valid {
			if _, err := tx.ExecContext(ctx, `
				UPDATE failure_register_entries SET superseded_by = $1 WHERE id = $2
			`, entry.ID, previousID.String); err != nil {
				return fmt.Errorf("linking the superseded verdict to its reversal: %w", err)
			}
		}
		return nil
	})
	if err != nil {
		return FailureRegisterEntry{}, fmt.Errorf("datastore: %w", err)
	}
	return entry, nil
}

// GetFailureRegisterEntry reads one entry, whatever its status. A superseded
// or resolved entry is still readable — that is what makes the history worth
// keeping.
func (db *DB) GetFailureRegisterEntry(ctx context.Context, id string) (FailureRegisterEntry, error) {
	row := db.pool.QueryRowContext(ctx,
		`SELECT `+failureEntryColumns+` FROM failure_register_entries WHERE id = $1`, id)

	entry, err := scanFailureEntry(row)
	if errors.Is(err, sql.ErrNoRows) {
		return FailureRegisterEntry{}, ErrNotFound
	}
	if err != nil {
		// A malformed id is a not-found, not a server fault: the column is a
		// UUID and PostgreSQL rejects the cast before it looks anything up.
		if isInvalidUUID(err) {
			return FailureRegisterEntry{}, ErrNotFound
		}
		return FailureRegisterEntry{}, fmt.Errorf("datastore: reading a register entry: %w", err)
	}
	return entry, nil
}

// ReviseFailureEntry updates the diagnosis, the plan, the target date and who
// is on it. The verdict and the reason are immutable by construction — a
// reversal is a new verdict, recorded with RecordFailureVerdict.
func (db *DB) ReviseFailureEntry(ctx context.Context, id string, p ReviseFailureEntryParams) (FailureRegisterEntry, error) {
	current, err := db.GetFailureRegisterEntry(ctx, id)
	if err != nil {
		return FailureRegisterEntry{}, err
	}
	if current.Status != FailureStatusOpen {
		return FailureRegisterEntry{}, fmt.Errorf("datastore: this entry is %s; only a standing verdict can be revised", current.Status)
	}

	// The holder is validated against the merged result, because setting only
	// one half of it against a stored other half is legitimate.
	holderType, holderRef := current.HolderType, current.HolderRef
	if p.HolderType != nil {
		holderType = *p.HolderType
	}
	if p.HolderRef != nil {
		holderRef = *p.HolderRef
	}
	holderType, holderRef, err = resolveHolder(holderType, holderRef)
	if err != nil {
		return FailureRegisterEntry{}, err
	}

	row := db.pool.QueryRowContext(ctx, `
		UPDATE failure_register_entries
		SET diagnosis   = COALESCE($2, diagnosis),
		    plan        = COALESCE($3, plan),
		    evidence    = COALESCE($4, evidence),
		    target_date = COALESCE($5, target_date),
		    holder_type = $6,
		    holder_ref  = $7,
		    updated_at  = now()
		WHERE id = $1 AND status = 'open'
		RETURNING `+failureEntryColumns,
		id,
		stringPtrArg(p.Diagnosis), stringPtrArg(p.Plan), stringPtrArg(p.Evidence),
		nullTimePtr(p.TargetDate),
		nullStringPtr(holderType), nullStringPtr(holderRef),
	)
	entry, err := scanFailureEntry(row)
	if errors.Is(err, sql.ErrNoRows) {
		return FailureRegisterEntry{}, ErrNotFound
	}
	if err != nil {
		return FailureRegisterEntry{}, fmt.Errorf("datastore: revising a register entry: %w", err)
	}
	return entry, nil
}

// ResolveFailureEntry records that a standing verdict has been dealt with. The
// entry stays: journey 6 needs the direction of travel, which is unavailable
// if resolved entries vanish.
func (db *DB) ResolveFailureEntry(ctx context.Context, id, resolvedBy, note string) (FailureRegisterEntry, error) {
	if strings.TrimSpace(resolvedBy) == "" {
		return FailureRegisterEntry{}, errors.New("datastore: resolving needs to say who resolved it")
	}

	row := db.pool.QueryRowContext(ctx, `
		UPDATE failure_register_entries
		SET status = 'resolved', resolved_by = $2, resolved_at = now(),
		    resolution_note = $3, updated_at = now()
		WHERE id = $1 AND status = 'open'
		RETURNING `+failureEntryColumns,
		id, resolvedBy, nullStringPtr(strings.TrimSpace(note)))

	entry, err := scanFailureEntry(row)
	if errors.Is(err, sql.ErrNoRows) {
		// Either it does not exist, or it is not standing. Told apart so the
		// caller can say which, rather than reporting a confident nothing.
		if _, getErr := db.GetFailureRegisterEntry(ctx, id); getErr != nil {
			return FailureRegisterEntry{}, getErr
		}
		return FailureRegisterEntry{}, errors.New("datastore: only a standing verdict can be resolved")
	}
	if err != nil {
		if isInvalidUUID(err) {
			return FailureRegisterEntry{}, ErrNotFound
		}
		return FailureRegisterEntry{}, fmt.Errorf("datastore: resolving a register entry: %w", err)
	}
	return entry, nil
}

// ListFailureRegisterEntries reads the register, most recently raised first,
// with the total matching the filter.
func (db *DB) ListFailureRegisterEntries(ctx context.Context, f FailureRegisterFilter) ([]FailureRegisterEntry, int, error) {
	where := "WHERE 1=1"
	args := []any{}
	argN := 1

	if f.Status != "" {
		where += fmt.Sprintf(" AND status = $%d", argN)
		args = append(args, f.Status)
		argN++
	}
	if f.Verdict != "" {
		where += fmt.Sprintf(" AND verdict = $%d", argN)
		args = append(args, f.Verdict)
		argN++
	}
	if f.SubjectName != "" {
		where += fmt.Sprintf(" AND subject_name = $%d", argN)
		args = append(args, f.SubjectName)
		argN++
	}

	var total int
	if err := db.pool.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM failure_register_entries `+where, args...,
	).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("datastore: counting register entries: %w", err)
	}

	limit := f.Limit
	if limit <= 0 {
		limit = 50
	}
	offset := f.Offset
	if offset < 0 {
		offset = 0
	}
	args = append(args, limit, offset)

	query := `SELECT ` + failureEntryColumns + ` FROM failure_register_entries ` + where +
		fmt.Sprintf(` ORDER BY raised_at DESC, id LIMIT $%d OFFSET $%d`, argN, argN+1)

	rows, err := db.pool.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, 0, fmt.Errorf("datastore: listing register entries: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var out []FailureRegisterEntry
	for rows.Next() {
		entry, err := scanFailureEntry(rows)
		if err != nil {
			return nil, 0, fmt.Errorf("datastore: scanning a register entry: %w", err)
		}
		out = append(out, entry)
	}
	return out, total, rows.Err()
}

// ListFailureRegisterHistory returns every verdict ever recorded about one
// repo, newest first. This is where a reader sees that a scan called a
// cookbook incompatible, a person overruled it, and why.
func (db *DB) ListFailureRegisterHistory(ctx context.Context, gitRepoName string) ([]FailureRegisterEntry, error) {
	rows, err := db.pool.QueryContext(ctx,
		`SELECT `+failureEntryColumns+`
		 FROM failure_register_entries
		 WHERE subject_name = $1
		 ORDER BY raised_at DESC, id`, gitRepoName)
	if err != nil {
		return nil, fmt.Errorf("datastore: reading the register history for %q: %w", gitRepoName, err)
	}
	defer func() { _ = rows.Close() }()

	var out []FailureRegisterEntry
	for rows.Next() {
		entry, err := scanFailureEntry(rows)
		if err != nil {
			return nil, fmt.Errorf("datastore: scanning a register entry: %w", err)
		}
		out = append(out, entry)
	}
	return out, rows.Err()
}

// ListOpenFailureVerdicts returns the standing verdicts keyed on repo name,
// which is how the readiness evaluator consumes them.
//
// Only open entries: a superseded verdict has been reversed and a resolved one
// has been dealt with. Neither is anybody's current opinion.
func (db *DB) ListOpenFailureVerdicts(ctx context.Context) (map[string]StandingVerdict, error) {
	rows, err := db.pool.QueryContext(ctx, `
		SELECT subject_name, subject_type, cookbook_name, verdict, reason, raised_by, raised_at
		FROM failure_register_entries
		WHERE status = 'open'
	`)
	if err != nil {
		return nil, fmt.Errorf("datastore: reading the standing verdicts: %w", err)
	}
	defer func() { _ = rows.Close() }()

	out := map[string]StandingVerdict{}
	for rows.Next() {
		var v StandingVerdict
		if err := rows.Scan(&v.SubjectName, &v.SubjectType, &v.CookbookName, &v.Verdict, &v.Reason,
			&v.RaisedBy, &v.RaisedAt); err != nil {
			return nil, fmt.Errorf("datastore: scanning a standing verdict: %w", err)
		}
		out[v.SubjectName] = v
	}
	return out, rows.Err()
}

// FailureRegisterSummary reports how large the register is and which way it is
// moving over the last windowDays days.
func (db *DB) FailureRegisterSummary(ctx context.Context, windowDays int) (FailureRegisterSummary, error) {
	if windowDays <= 0 {
		windowDays = 7
	}
	since := time.Now().AddDate(0, 0, -windowDays)

	s := FailureRegisterSummary{WindowDays: windowDays}
	err := db.pool.QueryRowContext(ctx, `
		SELECT
			COUNT(*) FILTER (WHERE status = 'open'),
			COUNT(*) FILTER (WHERE status = 'open' AND verdict = 'broken'),
			COUNT(*) FILTER (WHERE status = 'open' AND verdict = 'not_broken'),
			COUNT(*) FILTER (WHERE status = 'open' AND holder_ref IS NULL),
			COUNT(*) FILTER (WHERE status = 'open' AND target_date IS NOT NULL AND target_date < CURRENT_DATE),
			COUNT(*) FILTER (WHERE raised_at >= $1),
			COUNT(*) FILTER (WHERE resolved_at IS NOT NULL AND resolved_at >= $1),
			COUNT(*) FILTER (WHERE verdict = 'broken'),
			COUNT(*) FILTER (WHERE verdict = 'not_broken'),
			COUNT(*) FILTER (WHERE status = 'resolved')
		FROM failure_register_entries
	`, since).Scan(
		&s.Open, &s.OpenBroken, &s.OpenNotBroken, &s.OpenWithoutHolder, &s.OpenOverdue,
		&s.RaisedInWindow, &s.ResolvedInWindow,
		&s.TotalBroken, &s.TotalNotBroken, &s.Resolved,
	)
	if err != nil {
		return FailureRegisterSummary{}, fmt.Errorf("datastore: summarising the register: %w", err)
	}
	return s, nil
}

// stringPtrArg turns an optional string into a query argument: nil means
// "leave whatever is stored alone", which the UPDATE reads through COALESCE.
func stringPtrArg(s *string) any {
	if s == nil {
		return nil
	}
	if strings.TrimSpace(*s) == "" {
		// An explicitly emptied field is a clear, not a no-op. COALESCE would
		// read NULL as "leave it", so an empty string is stored instead and
		// read back through the COALESCE in the projection.
		return ""
	}
	return *s
}

// isInvalidUUID reports whether an error is PostgreSQL rejecting a malformed
// UUID literal, which is a not-found rather than a fault.
func isInvalidUUID(err error) bool {
	return err != nil && strings.Contains(err.Error(), "invalid input syntax for type uuid")
}
