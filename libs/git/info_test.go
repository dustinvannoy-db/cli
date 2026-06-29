package git

import (
	"context"
	"sync/atomic"
	"testing"

	"github.com/databricks/cli/libs/dbr"
	"github.com/databricks/cli/libs/testserver"
	"github.com/databricks/databricks-sdk-go"
	"github.com/databricks/databricks-sdk-go/service/workspace"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	// Bundle root passed to FetchRepositoryInfo: a subdirectory of the git folder.
	testBundleRoot = "/Workspace/Users/test/bundle-examples/dabs_in_ws_bundle"
	// Git folder path as get-status returns it (without the /Workspace prefix).
	testGitFolderRaw = "/Users/test/bundle-examples"
	// Expected worktree root after ensureWorkspacePrefix is applied.
	testWorktreeRoot = "/Workspace/Users/test/bundle-examples"
	testRepoID       = int64(2884540697170475)
	testOriginURL    = "https://github.com/databricks/bundle-examples.git"
)

func newTestWorkspaceClient(t *testing.T, server *testserver.Server) *databricks.WorkspaceClient {
	t.Helper()
	w, err := databricks.NewWorkspaceClient(&databricks.Config{
		Host:  server.URL,
		Token: "testtoken",
	})
	require.NoError(t, err)
	return w
}

// runtimeContext forces the in-workspace API branch of FetchRepositoryInfo
// without needing a real /databricks directory on the test host.
func runtimeContext(t *testing.T) context.Context {
	return dbr.MockRuntime(t.Context(), dbr.Environment{IsDbr: true, Version: "15.4"})
}

// FetchRepositoryInfo reads git provenance from the workspace get-status API when
// running on DBR. What get-status returns depends on the kind of in-workspace Git
// folder the bundle lives in (see the backend tree-node contract):
//
//   - Classic Repos (/Repos/...) and workspace Git folders (/Workspace/...) are the
//     same NodeType.Project backend and return full git info inline, so the Repos API
//     is not consulted.
//   - New in-workspace Git folders (git-in-dataplane) are a plain folder promoted to a
//     repo by a .git child; get-status returns only id+path, so the missing
//     branch/commit/url are recovered from the Repos API by id.
func TestFetchRepositoryInfoAPI(t *testing.T) {
	fullGitInfo := map[string]any{
		"id":             testRepoID,
		"path":           testGitFolderRaw,
		"branch":         "main",
		"head_commit_id": "abc123",
		"url":            testOriginURL,
	}
	idOnlyGitInfo := map[string]any{
		"id":   testRepoID,
		"path": testGitFolderRaw,
	}

	tests := []struct {
		name string
		// gitInfo is the git_info object returned by get-status; nil means it is omitted.
		gitInfo map[string]any
		// repoResponse, when set, is returned by GET /api/2.0/repos/{repo_id}.
		repoResponse *workspace.GetRepoResponse
		// repoStatusCode, when set and repoResponse is nil, is the Repos API error status.
		repoStatusCode int

		wantReposCalled  bool
		wantBranch       string
		wantCommit       string
		wantOriginURL    string
		wantWorktreeRoot string
	}{
		{
			name:             "classic repo returns full git info inline",
			gitInfo:          fullGitInfo,
			wantReposCalled:  false,
			wantBranch:       "main",
			wantCommit:       "abc123",
			wantOriginURL:    testOriginURL,
			wantWorktreeRoot: testWorktreeRoot,
		},
		{
			// Same NodeType.Project backend as a classic Repo, just relocated under
			// /Workspace; get-status returns the same full git info inline.
			name:             "workspace git folder returns full git info inline",
			gitInfo:          fullGitInfo,
			wantReposCalled:  false,
			wantBranch:       "main",
			wantCommit:       "abc123",
			wantOriginURL:    testOriginURL,
			wantWorktreeRoot: testWorktreeRoot,
		},
		{
			name:    "git-in-dataplane folder falls back to Repos API",
			gitInfo: idOnlyGitInfo,
			repoResponse: &workspace.GetRepoResponse{
				Id:           testRepoID,
				Branch:       "main",
				HeadCommitId: "d53214abc",
				Url:          testOriginURL,
				Provider:     "gitHub",
				Path:         testGitFolderRaw,
			},
			wantReposCalled:  true,
			wantBranch:       "main",
			wantCommit:       "d53214abc",
			wantOriginURL:    testOriginURL,
			wantWorktreeRoot: testWorktreeRoot,
		},
		{
			// A remoteless dataplane folder has no origin URL even from the Repos API,
			// which back-fills branch/commit live but leaves url empty.
			name:    "remoteless dataplane folder recovers branch and commit but not url",
			gitInfo: idOnlyGitInfo,
			repoResponse: &workspace.GetRepoResponse{
				Id:           testRepoID,
				Branch:       "main",
				HeadCommitId: "d53214abc",
				Url:          "",
				Path:         testGitFolderRaw,
			},
			wantReposCalled:  true,
			wantBranch:       "main",
			wantCommit:       "d53214abc",
			wantOriginURL:    "",
			wantWorktreeRoot: testWorktreeRoot,
		},
		{
			// A failed Repos lookup must not fail the deploy: the worktree root stays
			// set and provenance stays empty, with no error.
			name:             "Repos lookup failure degrades gracefully",
			gitInfo:          idOnlyGitInfo,
			repoStatusCode:   404,
			wantReposCalled:  true,
			wantBranch:       "",
			wantCommit:       "",
			wantOriginURL:    "",
			wantWorktreeRoot: testWorktreeRoot,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := testserver.New(t)
			var reposCalled atomic.Bool

			server.Handle("GET", "/api/2.0/workspace/get-status", func(_ testserver.Request) any {
				body := map[string]any{}
				if tt.gitInfo != nil {
					body["git_info"] = tt.gitInfo
				}
				return testserver.Response{Body: body}
			})
			server.Handle("GET", "/api/2.0/repos/{repo_id}", func(_ testserver.Request) any {
				reposCalled.Store(true)
				if tt.repoResponse != nil {
					return testserver.Response{Body: *tt.repoResponse}
				}
				return testserver.Response{StatusCode: tt.repoStatusCode, Body: map[string]string{"message": "not found"}}
			})

			info, err := FetchRepositoryInfo(runtimeContext(t), testBundleRoot, newTestWorkspaceClient(t, server))
			require.NoError(t, err)
			assert.Equal(t, tt.wantReposCalled, reposCalled.Load())
			assert.Equal(t, tt.wantBranch, info.CurrentBranch)
			assert.Equal(t, tt.wantCommit, info.LatestCommit)
			assert.Equal(t, tt.wantOriginURL, info.OriginURL)
			assert.Equal(t, tt.wantWorktreeRoot, info.WorktreeRoot)
		})
	}
}
