package ckg

import (
	"context"
	"fmt"
	"os/exec"
	"regexp"
	"strings"

	"github.com/0xmhha/coding-agent/tools/cks-mcp/internal/types"
)

// gitLogSeparator is unlikely to appear in commit messages and keeps the
// parser cheap. We pass it via --format=%H<sep>%s<sep>%ai<sep>%an.
const gitLogSeparator = "\x1f"

// HistoryAnalyzer pulls per-symbol commit history from git.
//
// We call git out of process because go-git pulls in significant code and
// we only need read-only log queries.
type HistoryAnalyzer struct {
	repoDir string
	limit   int
}

// NewHistoryAnalyzer constructs an analyzer rooted at repoDir.
// limit caps the number of entries fetched per symbol (default 10).
func NewHistoryAnalyzer(repoDir string, limit int) *HistoryAnalyzer {
	if limit <= 0 {
		limit = 10
	}
	return &HistoryAnalyzer{repoDir: repoDir, limit: limit}
}

// History returns commits that touched the specified line range of filePath.
// Uses `git log -L startLine,endLine:filePath` which works even after renames.
func (h *HistoryAnalyzer) History(
	ctx context.Context, filePath string, startLine, endLine int,
) ([]types.SymbolHistoryEntry, error) {
	if startLine <= 0 || endLine <= 0 || endLine < startLine {
		return nil, nil
	}
	rangeArg := fmt.Sprintf("-L%d,%d:%s", startLine, endLine, filePath)
	cmd := exec.CommandContext(ctx,
		"git", "-C", h.repoDir,
		"log", rangeArg,
		"--no-patch",
		fmt.Sprintf("--format=%%H%s%%s%s%%ai%s%%an", gitLogSeparator, gitLogSeparator, gitLogSeparator),
		fmt.Sprintf("-%d", h.limit),
	)
	out, err := cmd.Output()
	if err != nil {
		// Common failure modes: outside-repo, untracked file. Don't escalate.
		return nil, nil
	}
	return parseHistoryLines(out), nil
}

// HistoryByFile returns the recent commit list for the file path, following
// renames. Used when we want a coarser history than line range.
func (h *HistoryAnalyzer) HistoryByFile(
	ctx context.Context, filePath string,
) ([]types.SymbolHistoryEntry, error) {
	cmd := exec.CommandContext(ctx,
		"git", "-C", h.repoDir,
		"log", "--follow",
		fmt.Sprintf("--format=%%H%s%%s%s%%ai%s%%an", gitLogSeparator, gitLogSeparator, gitLogSeparator),
		fmt.Sprintf("-%d", h.limit),
		"--", filePath,
	)
	out, err := cmd.Output()
	if err != nil {
		return nil, nil
	}
	return parseHistoryLines(out), nil
}

// SummarizeHistory returns a short multi-line string for inclusion in CKV
// search results or impact reports.
func SummarizeHistory(entries []types.SymbolHistoryEntry, max int) string {
	if max <= 0 || max > len(entries) {
		max = len(entries)
	}
	var sb strings.Builder
	for i := 0; i < max; i++ {
		e := entries[i]
		date := e.CommitDate
		if len(date) >= 10 {
			date = date[:10]
		}
		ct := e.ChangeType
		if ct == "" {
			ct = "change"
		}
		fmt.Fprintf(&sb, "%s: [%s] %s (%s)\n", date, ct, e.CommitMessage, e.Author)
	}
	return strings.TrimSpace(sb.String())
}

// --- Parsing helpers ---

func parseHistoryLines(stdout []byte) []types.SymbolHistoryEntry {
	var out []types.SymbolHistoryEntry
	for _, raw := range strings.Split(string(stdout), "\n") {
		raw = strings.TrimSpace(raw)
		if raw == "" {
			continue
		}
		parts := strings.SplitN(raw, gitLogSeparator, 4)
		if len(parts) < 4 {
			// `git log -L` interleaves diff hunks; skip anything that isn't
			// our well-formed metadata line.
			continue
		}
		entry := types.SymbolHistoryEntry{
			CommitHash:    strings.TrimSpace(parts[0]),
			CommitMessage: strings.TrimSpace(parts[1]),
			CommitDate:    strings.TrimSpace(parts[2]),
			Author:        strings.TrimSpace(parts[3]),
		}
		entry.ChangeType = classifyCommitMessage(entry.CommitMessage)
		out = append(out, entry)
	}
	return out
}

var (
	reBugfix   = regexp.MustCompile(`(?i)\b(fix|bug|patch|hotfix|repair|resolve)\b`)
	reFeature  = regexp.MustCompile(`(?i)\b(add|feat|feature|implement|introduce|new)\b`)
	reRefactor = regexp.MustCompile(`(?i)\b(refactor|rename|move|cleanup|simplif|extract|inline)\b`)
	reTest     = regexp.MustCompile(`(?i)\b(test|spec|coverage|mock|fixture)\b`)
)

// classifyCommitMessage returns one of "bugfix" | "feature" | "refactor"
// | "test" | "change". Order matters — bugfix wins over feature when both
// keywords are present.
func classifyCommitMessage(msg string) string {
	switch {
	case reBugfix.MatchString(msg):
		return "bugfix"
	case reFeature.MatchString(msg):
		return "feature"
	case reRefactor.MatchString(msg):
		return "refactor"
	case reTest.MatchString(msg):
		return "test"
	default:
		return "change"
	}
}
