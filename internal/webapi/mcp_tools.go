// Copyright 2025 Chef Migration Metrics Authors
// SPDX-License-Identifier: Apache-2.0

package webapi

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"github.com/trickyearlobe-chef/chef-migration-metrics/internal/auth"
)

// What an assistant is offered. See journeys/agent-access.md and the note at
// the top of mcp.go for why this is a chosen list rather than a generated one.
//
// Each entry corresponds to something the journey says is needed to diagnose a
// failing cookbook: the failure itself with the trace under it, the source of
// the file it points at, our classification of the static findings rather than
// the raw tool's, what happened on a real machine, the shape of the estate
// before pulling any of it, and the one write there is.
//
// A tool names a request and nothing more. It does not query, and it does not
// decide what a caller may see: mcpDispatchInward sends the request it named
// through the same mux a browser reaches, so access, scope and bounds are the
// ones already enforced everywhere else.

const (
	mcpFindCookbooksTool   = "cmm_find_cookbooks"
	mcpCookbookTool        = "cmm_cookbook"
	mcpBlockingFindingTool = "cmm_blocking_findings"
	mcpRunFailuresTool     = "cmm_run_failures"
	mcpSourceFileTool      = "cmm_source_file"
	mcpKitchenResultTool   = "cmm_kitchen_result"
	mcpEstateShapeTool     = "cmm_estate_shape"
	mcpRecordFindingTool   = "cmm_record_finding"
)

// The bounds this surface applies, which are its own and smaller than the API's.
//
// The API caps a page at 500 rows and a file at a megabyte. Both are bounded and
// both are far more than an assistant can hold: one such answer fills its whole
// working room, and what follows is not an error anybody sees but an assistant
// gone vague. So a caller that asks for more here gets less, and a caller that
// asks for nothing gets a small answer rather than a large one.
const (
	mcpDefaultPerPage = 20
	mcpMaxPerPage     = 100

	// mcpMaxAnswerBytes is the most that will be handed over in one piece.
	// Past it the call is refused and the refusal says how to narrow, which is
	// something an assistant can act on — a truncated answer is not.
	mcpMaxAnswerBytes = 96 * 1024

	// mcpDefaultMaxLines is how much of a file comes back when nobody said.
	mcpDefaultMaxLines = 400
)

// mcpTool is one capability, as an assistant sees it, plus the request it makes.
type mcpTool struct {
	// name is what an assistant calls it. Prefixed, because an assistant holds
	// several surfaces at once and an unprefixed "find_cookbooks" is a
	// collision waiting to happen.
	name string
	// description is the whole of what an assistant has to go on when picking
	// from the list. It says what the tool answers, and — where it matters —
	// what it does NOT, because the journey's field reports are of a tool being
	// passed over rather than misread.
	description string
	// schema describes the arguments. Always an object; the protocol allows
	// nothing else.
	schema map[string]any
	// write marks the one tool that changes something. Offered only to a
	// credential whose owner chose write when they made it.
	write bool
	// probePath is a concrete address this tool reaches, held against the mux
	// by a test so a tool cannot come to name something nothing serves.
	probePath string
	// narrowing is what a caller is told to do when an answer is too big to
	// hand over. Named per tool because "ask for fewer" is different advice
	// from "name a cookbook".
	narrowing string
	// build turns the arguments into the request to make.
	build func(args map[string]any) (method, path string, body []byte, err error)
	// shape adjusts the answer before an assistant sees it. Only one tool has
	// one, and it narrows rather than rewrites.
	shape func(args map[string]any, raw []byte) ([]byte, error)
}

// shapeAnswer applies the tool's own narrowing, where it has one.
func (t mcpTool) shapeAnswer(args map[string]any, raw []byte) ([]byte, error) {
	if t.shape == nil {
		return raw, nil
	}
	return t.shape(args, raw)
}

// mcpTools is the list, in the order an assistant reads it: find something,
// look at it closely, then the three kinds of evidence, then the overview, then
// the one write.
var mcpTools = []mcpTool{
	{
		name: mcpFindCookbooksTool,
		description: "Find cookbooks in this estate, narrowed and a page at a time. Start " +
			"here when you have a name, a symptom or an organisation rather than one " +
			"cookbook. Answers a page with the total beside it, so you can tell a whole " +
			"answer from the first slice of one. Use cmm_cookbook for one cookbook in full.",
		schema: mcpSchema(map[string]any{
			"name": mcpStringArg("Match on the cookbook name. A fragment matches."),
			"compatibility": mcpStringArg("Whether it is compatible with the target Chef " +
				"version — for example \"incompatible\"."),
			"cookstyle_status":    mcpStringArg("The state of the static check for it."),
			"tk_status":           mcpStringArg("The state of the last real-machine run."),
			"organisation":        mcpStringArg("Limit to one Chef organisation."),
			"owner":               mcpStringArg("Limit to one owner."),
			"unowned":             mcpStringArg("\"true\" for the ones nobody owns."),
			"active":              mcpStringArg("\"true\" for the ones a node actually uses."),
			"download_status":     mcpStringArg("Whether we hold a copy of the source."),
			"target_chef_version": mcpStringArg("The version being moved to. Defaults to this deployment's."),
			"page":                mcpPageArg(),
			"per_page":            mcpPerPageArg(),
		}, nil),
		probePath: "/api/v1/cookbooks",
		narrowing: "give it a name, a compatibility or an organisation, or ask for a smaller per_page.",
		build: func(args map[string]any) (string, string, []byte, error) {
			q := url.Values{}
			mcpCopyFilters(args, q, "name", "compatibility", "cookstyle_status", "tk_status",
				"organisation", "owner", "unowned", "active", "download_status",
				"target_chef_version")
			mcpPage(args, q)
			return http.MethodGet, "/api/v1/cookbooks?" + q.Encode(), nil, nil
		},
	},
	{
		name: mcpCookbookTool,
		description: "Everything held about one cookbook: the versions in use, our verdict " +
			"on whether it can move, and what that verdict rests on. Needs the exact name — " +
			"use cmm_find_cookbooks if you only have a fragment.",
		schema: mcpSchema(map[string]any{
			"name": mcpStringArg("The exact cookbook name."),
		}, []string{"name"}),
		probePath: "/api/v1/cookbooks/example-cookbook",
		narrowing: "there is nothing narrower than one cookbook; read it in pieces with the other tools.",
		build: func(args map[string]any) (string, string, []byte, error) {
			name, err := mcpRequired(args, "name")
			if err != nil {
				return "", "", nil, err
			}
			return http.MethodGet, "/api/v1/cookbooks/" + url.PathEscape(name), nil, nil
		},
	},
	{
		name: mcpBlockingFindingTool,
		description: "Our classification of the static findings — which actually block the " +
			"move to the target version and which are tidying — grouped by finding, with " +
			"how many things each one hits. This is OUR judgement, not the raw tool's " +
			"output, and ranking by the raw output puts the wrong things first. Name a cop " +
			"to get the things that finding hits instead of the grouping.",
		schema: mcpSchema(map[string]any{
			"cop": mcpStringArg("A single finding, by its cop name. Answers what it hits " +
				"rather than the grouping."),
			"classification": mcpStringArg("Limit to findings we classified a given way — " +
				"for example the blocking ones."),
			"triggered_only":      mcpStringArg("\"true\" for only the findings something actually triggers."),
			"source":              mcpStringArg("Where the finding came from."),
			"target_chef_version": mcpStringArg("The version being moved to. Defaults to this deployment's."),
			"page":                mcpPageArg(),
			"per_page":            mcpPerPageArg(),
		}, nil),
		probePath: "/api/v1/cookstyle/cops",
		narrowing: "ask for a smaller per_page, or filter by classification.",
		build: func(args map[string]any) (string, string, []byte, error) {
			q := url.Values{}
			mcpPage(args, q)
			if cop := mcpString(args, "cop"); cop != "" {
				mcpCopyFilters(args, q, "source", "target_chef_version")
				return http.MethodGet, "/api/v1/cookstyle/cops/" + url.PathEscape(cop) +
					"/cookbooks?" + q.Encode(), nil, nil
			}
			mcpCopyFilters(args, q, "classification", "triggered_only", "source",
				"target_chef_version")
			return http.MethodGet, "/api/v1/cookstyle/cops?" + q.Encode(), nil, nil
		},
	},
	{
		name: mcpRunFailuresTool,
		description: "What came back from real Chef runs: the error, the trace under it, " +
			"which cookbook was running and which machine it happened on. Answers failures " +
			"unless you ask for another status. This is the failure itself rather than a " +
			"verdict about it, and it is the thing to read before deciding why something " +
			"is broken.",
		schema: mcpSchema(map[string]any{
			"cookbook":        mcpStringArg("Limit to runs involving one cookbook."),
			"node":            mcpStringArg("Limit to one machine."),
			"organisation":    mcpStringArg("Limit to one Chef organisation, as delivered."),
			"chef_version":    mcpStringArg("Limit to one Chef version."),
			"failure_message": mcpStringArg("Match on the text of the failure."),
			"status": mcpStringArg("\"failure\" (the default) or \"success\". A comma-separated " +
				"list takes both."),
			"since":    mcpStringArg("Only runs that ended after this time, as RFC 3339."),
			"until":    mcpStringArg("Only runs that ended before this time, as RFC 3339."),
			"page":     mcpPageArg(),
			"per_page": mcpPerPageArg(),
		}, nil),
		probePath: "/api/v1/run-events/runs",
		narrowing: "name a cookbook, a machine or a time window, or ask for a smaller per_page.",
		build: func(args map[string]any) (string, string, []byte, error) {
			q := url.Values{}
			mcpCopyFilters(args, q, "cookbook", "node", "organisation", "chef_version",
				"failure_message", "since", "until")
			// Failures unless asked otherwise. This tool is named for what it
			// is for, and an assistant that got every successful run back would
			// spend its whole answer on runs nobody is asking about.
			status := mcpString(args, "status")
			if status == "" {
				status = "failure"
			}
			q.Set("status", status)
			mcpPage(args, q)
			return http.MethodGet, "/api/v1/run-events/runs?" + q.Encode(), nil, nil
		},
	},
	{
		name: mcpSourceFileTool,
		description: "The contents of a file out of a repository we already hold, so you can " +
			"read the recipe a failure points at rather than guessing it from its name. " +
			"With no path it lists what is in the repository. Long files come back a window " +
			"at a time — ask for the lines around the one the trace names.",
		schema: mcpSchema(map[string]any{
			"repo": mcpStringArg("The repository name, as this service holds it."),
			"path": mcpStringArg("The file to read, relative to the repository root. " +
				"Omit to list what is there."),
			"from_line": mcpNumberArg("The first line to return. Counts from 1."),
			"max_lines": mcpNumberArg(fmt.Sprintf(
				"How many lines to return. %d when nobody says.", mcpDefaultMaxLines)),
		}, []string{"repo"}),
		probePath: "/api/v1/git-repos/example-cookbook/files/content",
		narrowing: "ask for a window with from_line and max_lines.",
		build: func(args map[string]any) (string, string, []byte, error) {
			repo, err := mcpRequired(args, "repo")
			if err != nil {
				return "", "", nil, err
			}
			base := "/api/v1/git-repos/" + url.PathEscape(repo) + "/files"
			path := mcpString(args, "path")
			if path == "" {
				return http.MethodGet, base, nil, nil
			}
			q := url.Values{"path": {path}}
			return http.MethodGet, base + "/content?" + q.Encode(), nil, nil
		},
		shape: mcpWindowFileContent,
	},
	{
		name: mcpKitchenResultTool,
		description: "What happened when a cookbook was last run on a real machine. This " +
			"outranks the static check: a cookbook the static check dislikes can still " +
			"converge, and one it passes can still fail. With no cookbook named it answers " +
			"everything we have run, which is usually too much — name one.",
		schema: mcpSchema(map[string]any{
			"cookbook": mcpStringArg("The exact cookbook name."),
		}, nil),
		probePath: "/api/v1/kitchen/analysis/cookbooks",
		narrowing: "name a cookbook.",
		build: func(args map[string]any) (string, string, []byte, error) {
			base := "/api/v1/kitchen/analysis/cookbooks"
			if name := mcpString(args, "cookbook"); name != "" {
				return http.MethodGet, base + "/" + url.PathEscape(name), nil, nil
			}
			return http.MethodGet, base, nil, nil
		},
	},
	{
		name: mcpEstateShapeTool,
		description: "How many, grouped how — the counts behind one of the overview screens, " +
			"without pulling the things they count. Ask this before fetching anything: it " +
			"is how you find out whether a question is about six cookbooks or six thousand.",
		schema: mcpSchema(map[string]any{
			"view": map[string]any{
				"type":        "string",
				"description": "Which overview. " + mcpEstateViewSummary(),
				"enum":        mcpEstateViewNames(),
			},
			"organisation": mcpStringArg("Limit to one Chef organisation, where the view takes one."),
			"owner":        mcpStringArg("Limit to one owner, where the view takes one."),
			"unowned":      mcpStringArg("\"true\" for the ones nobody owns, where the view takes it."),
		}, []string{"view"}),
		probePath: "/api/v1/dashboard/readiness",
		narrowing: "these are counts; if one is too big to read, the view itself is wrong for the question.",
		build: func(args map[string]any) (string, string, []byte, error) {
			name, err := mcpRequired(args, "view")
			if err != nil {
				return "", "", nil, err
			}
			view, ok := mcpEstateViews[name]
			if !ok {
				return "", "", nil, fmt.Errorf(
					"there is no view called %q. The ones there are: %s",
					name, strings.Join(mcpEstateViewNames(), ", "))
			}
			q := url.Values{}
			mcpCopyFilters(args, q, view.filters...)
			if len(q) == 0 {
				return http.MethodGet, view.path, nil, nil
			}
			return http.MethodGet, view.path + "?" + q.Encode(), nil, nil
		},
	},
	{
		name: mcpRecordFindingTool,
		description: "Record a finding in the register of failures: our verdict on whether " +
			"one thing is really broken, and why. Nothing here is overwritten, and the " +
			"entry is marked as having come from a tool rather than from a person, so a " +
			"later reader can weigh it accordingly. Offered only to a credential its owner " +
			"created with write access.",
		write: true,
		schema: mcpSchema(map[string]any{
			"subject_name": mcpStringArg("What the entry is about: the repository where the " +
				"fix is made, or the cookbook itself where no repository has been collected."),
			"subject_type":  mcpStringArg("Whether subject_name is a repository or a cookbook."),
			"cookbook_name": mcpStringArg("What the failure is called at standup."),
			"verdict":       mcpStringArg("\"broken\" or \"not_broken\"."),
			"reason": mcpStringArg("Why. A verdict with no reason is an opinion; the reason " +
				"is what lets a later reader judge whether it still holds."),
			"evidence":    mcpStringArg("What the verdict rests on."),
			"diagnosis":   mcpStringArg("What is actually wrong."),
			"plan":        mcpStringArg("What is going to be done about it."),
			"target_date": mcpStringArg("When, as YYYY-MM-DD."),
			"holder_type": mcpStringArg("Whether a person or a ticket is carrying it."),
			"holder_ref":  mcpStringArg("Which person, or which ticket."),
		}, []string{"subject_name", "subject_type", "cookbook_name", "verdict", "reason"}),
		probePath: "/api/v1/failure-register",
		narrowing: "this records one entry; there is nothing to narrow.",
		build: func(args map[string]any) (string, string, []byte, error) {
			// Passed through as sent, and judged by the handler that judges
			// every other caller. Validating here would be a second set of
			// rules to drift out of step with the first.
			body, err := json.Marshal(args)
			if err != nil {
				return "", "", nil, errors.New("those arguments could not be encoded")
			}
			return http.MethodPost, failureRegisterPath, body, nil
		},
	},
}

// mcpEstateView is one overview, and the narrowing that overview really reads.
//
// Named per view rather than passed through wholesale: three of these take an
// owner and not an organisation, and a parameter a handler ignores is worse
// than one it refuses — an assistant reports a filtered count that was never
// filtered.
type mcpEstateView struct {
	path    string
	filters []string
	says    string
}

var mcpEstateViews = map[string]mcpEstateView{
	"readiness": {"/api/v1/dashboard/readiness",
		[]string{"organisation", "owner", "unowned"},
		"how much of the estate is ready to move"},
	"cookbook_compatibility": {"/api/v1/dashboard/cookbook-compatibility",
		[]string{"organisation", "owner", "unowned"},
		"cookbooks by whether they can move"},
	"git_repo_compatibility": {"/api/v1/dashboard/git-repo-compatibility",
		[]string{"owner", "unowned"},
		"repositories by whether they can move"},
	"test_kitchen_compatibility": {"/api/v1/dashboard/test-kitchen-compatibility",
		[]string{"owner", "unowned"},
		"what happened on real machines, counted"},
	"version_distribution": {"/api/v1/dashboard/version-distribution",
		[]string{"organisation", "owner", "unowned"},
		"machines by the Chef version they run"},
	"platform_distribution": {"/api/v1/dashboard/platform-distribution",
		[]string{"organisation", "owner", "unowned"},
		"machines by operating system"},
	"remediation_summary": {"/api/v1/remediation/summary",
		[]string{"organisation", "owner", "unowned"},
		"how much work the remaining fixes add up to"},
}

// mcpEstateViewNames lists the views in a stable order, so the description an
// assistant reads does not change between calls for no reason.
func mcpEstateViewNames() []string {
	return []string{
		"readiness", "cookbook_compatibility", "git_repo_compatibility",
		"test_kitchen_compatibility", "version_distribution", "platform_distribution",
		"remediation_summary",
	}
}

// mcpEstateViewSummary says what each view answers, in one sentence, because an
// assistant choosing between them has only this to go on.
func mcpEstateViewSummary() string {
	parts := make([]string, 0, len(mcpEstateViews))
	for _, name := range mcpEstateViewNames() {
		parts = append(parts, name+" — "+mcpEstateViews[name].says)
	}
	return strings.Join(parts, "; ") + "."
}

// mcpOfferedTools is the list this caller is shown.
//
// A tool somebody can see and cannot use is one an assistant keeps reaching
// for, and having already told its user it would. So the write is not listed
// for a credential that cannot make it — and, separately, not permitted either,
// because a listing is a courtesy and not a control.
func mcpOfferedTools(req *http.Request) []any {
	mayWrite := mcpMayWrite(auth.SessionFromContext(req.Context()))
	offered := make([]any, 0, len(mcpTools))
	for _, tool := range mcpTools {
		if tool.write && !mayWrite {
			continue
		}
		offered = append(offered, map[string]any{
			"name":        tool.name,
			"description": tool.description,
			"inputSchema": tool.schema,
		})
	}
	return offered
}

// mcpToolByName finds one.
func mcpToolByName(name string) (mcpTool, bool) {
	for _, tool := range mcpTools {
		if tool.name == name {
			return tool, true
		}
	}
	return mcpTool{}, false
}

// ---------------------------------------------------------------------------
// Arguments
// ---------------------------------------------------------------------------

// mcpSchema builds the object schema a tool's arguments are described by.
func mcpSchema(properties map[string]any, required []string) map[string]any {
	schema := map[string]any{
		"type":       "object",
		"properties": properties,
	}
	if len(required) > 0 {
		schema["required"] = required
	}
	return schema
}

func mcpStringArg(description string) map[string]any {
	return map[string]any{"type": "string", "description": description}
}

func mcpNumberArg(description string) map[string]any {
	return map[string]any{"type": "integer", "minimum": 1, "description": description}
}

func mcpPageArg() map[string]any {
	return map[string]any{
		"type": "integer", "minimum": 1, "default": defaultPage,
		"description": "Which page. Counts from 1. The answer says how many there are in all.",
	}
}

func mcpPerPageArg() map[string]any {
	return map[string]any{
		"type": "integer", "minimum": 1,
		"default": mcpDefaultPerPage, "maximum": mcpMaxPerPage,
		"description": fmt.Sprintf(
			"How many to return, at most %d. Asking for more is not refused — it returns %d.",
			mcpMaxPerPage, mcpMaxPerPage),
	}
}

// mcpString reads one argument as text. Numbers and booleans are accepted as
// what they look like, because an assistant sends what its own schema reading
// suggested and being strict here refuses calls the service would have taken.
func mcpString(args map[string]any, key string) string {
	switch v := args[key].(type) {
	case string:
		return strings.TrimSpace(v)
	case bool:
		return strconv.FormatBool(v)
	case float64:
		return strconv.FormatFloat(v, 'f', -1, 64)
	case json.Number:
		return v.String()
	default:
		return ""
	}
}

// mcpInt reads one argument as a whole number, reporting whether it was there.
func mcpInt(args map[string]any, key string) (int, bool) {
	switch v := args[key].(type) {
	case float64:
		return int(v), true
	case string:
		n, err := strconv.Atoi(strings.TrimSpace(v))
		return n, err == nil
	default:
		return 0, false
	}
}

// mcpRequired reads an argument the tool cannot work without, and says which
// one is missing rather than sending a request that will be refused for
// reasons an assistant cannot connect to what it sent.
func mcpRequired(args map[string]any, key string) (string, error) {
	if v := mcpString(args, key); v != "" {
		return v, nil
	}
	return "", fmt.Errorf("%s is needed and was not given", key)
}

// mcpCopyFilters passes the named arguments through to the query, skipping the
// ones nobody set. Only what the address behind the tool really reads is named
// at each call site.
func mcpCopyFilters(args map[string]any, q url.Values, names ...string) {
	for _, name := range names {
		if v := mcpString(args, name); v != "" {
			q.Set(name, v)
		}
	}
}

// mcpPage sets the page and how many are on it, applying this surface's own
// ceiling — which is lower than the API's, and applied whether or not the
// caller asked for a bound.
func mcpPage(args map[string]any, q url.Values) {
	page := defaultPage
	if n, ok := mcpInt(args, "page"); ok && n >= 1 {
		page = n
	}
	perPage := mcpDefaultPerPage
	if n, ok := mcpInt(args, "per_page"); ok && n >= 1 {
		perPage = n
	}
	if perPage > mcpMaxPerPage {
		perPage = mcpMaxPerPage
	}
	q.Set("page", strconv.Itoa(page))
	q.Set("per_page", strconv.Itoa(perPage))
}

// ---------------------------------------------------------------------------
// Narrowing a file
// ---------------------------------------------------------------------------

// mcpFileWindow is part of a file, saying which part.
//
// The three extra fields are the point: a window that did not say where it
// started, or how much it left behind, would read as the whole file — and an
// assistant reasoning about line 12 of what it thinks is a recipe, when it is
// holding lines 400 to 800, is confidently wrong rather than stuck.
type mcpFileWindow struct {
	Path       string `json:"path"`
	Encoding   string `json:"encoding"`
	Content    string `json:"content"`
	Size       int    `json:"size"`
	FromLine   int    `json:"from_line"`
	ToLine     int    `json:"to_line"`
	TotalLines int    `json:"total_lines"`
}

// mcpWindowFileContent cuts a long file down to the part that was asked for.
//
// Anything that is not a text file comes back untouched: there is no line to
// window on, and the API already refuses one over a megabyte.
func mcpWindowFileContent(args map[string]any, raw []byte) ([]byte, error) {
	var file fileContentResponse
	if err := json.Unmarshal(raw, &file); err != nil || file.Encoding != "text" {
		// A directory listing, or something not text. Neither has lines.
		return raw, nil
	}

	lines := strings.Split(file.Content, "\n")
	from := 1
	if n, ok := mcpInt(args, "from_line"); ok && n >= 1 {
		from = n
	}
	maxLines := mcpDefaultMaxLines
	if n, ok := mcpInt(args, "max_lines"); ok && n >= 1 {
		maxLines = n
	}
	if from == 1 && len(lines) <= maxLines {
		return raw, nil
	}
	if from > len(lines) {
		return nil, fmt.Errorf("%s has %d lines, so there is nothing at line %d",
			file.Path, len(lines), from)
	}

	to := from + maxLines - 1
	if to > len(lines) {
		to = len(lines)
	}
	return json.Marshal(mcpFileWindow{
		Path:       file.Path,
		Encoding:   file.Encoding,
		Content:    strings.Join(lines[from-1:to], "\n"),
		Size:       file.Size,
		FromLine:   from,
		ToLine:     to,
		TotalLines: len(lines),
	})
}
