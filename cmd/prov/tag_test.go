package main

import (
	"testing"
)

// V2 NOTE: Manual tagging feature is deferred
// These tests will be reimplemented when manual tagging is added back
// V2 uses commit windows for automatic association

// TestTagPromptWithCommit tests manually tagging a prompt to a commit
func TestTagPromptWithCommit(t *testing.T) {
	t.Skip("V2: Manual tagging deferred - will be reimplemented with commit window approach")
}

// TestTagPromptWithFile tests manually tagging a prompt to a file
func TestTagPromptWithFile(t *testing.T) {
	t.Skip("V2: Manual tagging deferred - will be reimplemented with commit window approach")
}

// TestTagNoPromptID tests error when no prompt ID provided
func TestTagNoPromptID(t *testing.T) {
	t.Skip("V2: Manual tagging deferred - will be reimplemented with commit window approach")
}

// TestTagNoTarget tests error when neither commit nor file specified
func TestTagNoTarget(t *testing.T) {
	t.Skip("V2: Manual tagging deferred - will be reimplemented with commit window approach")
}

// TestTagBothTargets tests error when both commit and file specified
func TestTagBothTargets(t *testing.T) {
	t.Skip("V2: Manual tagging deferred - will be reimplemented with commit window approach")
}

// TestTagNonexistentPrompt tests error when prompt doesn't exist
func TestTagNonexistentPrompt(t *testing.T) {
	t.Skip("V2: Manual tagging deferred - will be reimplemented with commit window approach")
}

// TestTagAppearsInBlame tests that manually tagged prompts show up in blame
func TestTagAppearsInBlame(t *testing.T) {
	t.Skip("V2: Manual tagging deferred - will be reimplemented with commit window approach")
}

// TestUntagPrompt tests removing a manual tag
func TestUntagPrompt(t *testing.T) {
	t.Skip("V2: Manual tagging deferred - will be reimplemented with commit window approach")
}

// TestUntagNoPromptID tests error when no prompt ID provided to untag
func TestUntagNoPromptID(t *testing.T) {
	t.Skip("V2: Manual tagging deferred - will be reimplemented with commit window approach")
}

// TestUntagNoCommit tests error when no commit provided to untag
func TestUntagNoCommit(t *testing.T) {
	t.Skip("V2: Manual tagging deferred - will be reimplemented with commit window approach")
}

// TestUntagNonexistentTag tests error when tag doesn't exist
func TestUntagNonexistentTag(t *testing.T) {
	t.Skip("V2: Manual tagging deferred - will be reimplemented with commit window approach")
}

// TestUntagAutoCorrelation tests that untag only removes manual tags, not auto-correlations
func TestUntagAutoCorrelation(t *testing.T) {
	t.Skip("V2: Manual tagging deferred - will be reimplemented with commit window approach")
}
