package network

import (
	"bytes"
	"encoding/json"
	"fmt"
	"github.com/stretchr/testify/assert"
	. "gitlab.com/gitlab-org/gitlab-ci-multi-runner/common"
	"io/ioutil"
	"net/http"
	"net/http/httptest"
	"os"
	"strconv"
	"strings"
	"testing"
)

var brokenCredentials = RunnerCredentials{
	URL: "broken",
}

var brokenConfig = RunnerConfig{
	RunnerCredentials: brokenCredentials,
}

func TestClients(t *testing.T) {
	c := GitLabClient{}
	c1, _ := c.getClient(RunnerCredentials{
		URL: "http://test/",
	})
	c2, _ := c.getClient(RunnerCredentials{
		URL: "http://test2/",
	})
	c4, _ := c.getClient(RunnerCredentials{
		URL:       "http://test/",
		TLSCAFile: "ca_file",
	})
	c5, _ := c.getClient(RunnerCredentials{
		URL:       "http://test/",
		TLSCAFile: "ca_file",
	})
	c6, c6err := c.getClient(brokenCredentials)
	assert.NotEqual(t, c1, c2)
	assert.NotEqual(t, c1, c4)
	assert.Equal(t, c4, c5)
	assert.Nil(t, c6)
	assert.Error(t, c6err)
}

func testGetBuildHandler(w http.ResponseWriter, r *http.Request, t *testing.T) {
	if r.URL.Path != "/ci/api/v1/builds/register.json" {
		w.WriteHeader(404)
		return
	}

	if r.Method != "POST" {
		w.WriteHeader(406)
		return
	}

	body, err := ioutil.ReadAll(r.Body)
	assert.NoError(t, err)

	var req map[string]interface{}
	err = json.Unmarshal(body, &req)
	assert.NoError(t, err)

	res := make(map[string]interface{})

	switch req["token"].(string) {
	case "valid":
		res["id"] = 10
	case "no-builds":
		w.WriteHeader(404)
		return
	case "invalid":
		w.WriteHeader(403)
		return
	default:
		w.WriteHeader(400)
		return
	}

	if r.Header.Get("Accept") != "application/json" {
		w.WriteHeader(400)
		return
	}

	output, err := json.Marshal(res)
	if err != nil {
		w.WriteHeader(500)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(201)
	w.Write(output)
}

func TestGetBuild(t *testing.T) {
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		testGetBuildHandler(w, r, t)
	}))
	defer s.Close()

	validToken := RunnerConfig{
		RunnerCredentials: RunnerCredentials{
			URL:   s.URL,
			Token: "valid",
		},
	}

	noBuildsToken := RunnerConfig{
		RunnerCredentials: RunnerCredentials{
			URL:   s.URL,
			Token: "no-builds",
		},
	}

	invalidToken := RunnerConfig{
		RunnerCredentials: RunnerCredentials{
			URL:   s.URL,
			Token: "invalid",
		},
	}

	c := GitLabClient{}

	res, ok := c.GetBuild(validToken)
	if assert.NotNil(t, res) {
		assert.NotEmpty(t, res.ID)
	}
	assert.True(t, ok)

	res, ok = c.GetBuild(noBuildsToken)
	assert.Nil(t, res)
	assert.True(t, ok, "If no builds, runner is healthy")

	res, ok = c.GetBuild(invalidToken)
	assert.Nil(t, res)
	assert.False(t, ok, "If token is invalid, the runner is unhealthy")

	res, ok = c.GetBuild(brokenConfig)
	assert.Nil(t, res)
	assert.False(t, ok)
}

func testRegisterRunnerHandler(w http.ResponseWriter, r *http.Request, t *testing.T) {
	if r.URL.Path != "/ci/api/v1/runners/register.json" {
		w.WriteHeader(404)
		return
	}

	if r.Method != "POST" {
		w.WriteHeader(406)
		return
	}

	body, err := ioutil.ReadAll(r.Body)
	assert.NoError(t, err)

	var req map[string]interface{}
	err = json.Unmarshal(body, &req)
	assert.NoError(t, err)

	res := make(map[string]interface{})

	switch req["token"].(string) {
	case "valid":
		if req["description"].(string) != "test" {
			w.WriteHeader(400)
			return
		}

		res["token"] = req["token"].(string)
	case "invalid":
		w.WriteHeader(403)
		return
	default:
		w.WriteHeader(400)
		return
	}

	if r.Header.Get("Accept") != "application/json" {
		w.WriteHeader(400)
		return
	}

	output, err := json.Marshal(res)
	if err != nil {
		w.WriteHeader(500)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(201)
	w.Write(output)
}

func TestRegisterRunner(t *testing.T) {
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		testRegisterRunnerHandler(w, r, t)
	}))
	defer s.Close()

	validToken := RunnerCredentials{
		URL:   s.URL,
		Token: "valid",
	}

	invalidToken := RunnerCredentials{
		URL:   s.URL,
		Token: "invalid",
	}

	otherToken := RunnerCredentials{
		URL:   s.URL,
		Token: "other",
	}

	c := GitLabClient{}

	res := c.RegisterRunner(validToken, "test", "tags")
	if assert.NotNil(t, res) {
		assert.Equal(t, validToken.Token, res.Token)
	}

	res = c.RegisterRunner(validToken, "invalid description", "tags")
	assert.Nil(t, res)

	res = c.RegisterRunner(invalidToken, "test", "tags")
	assert.Nil(t, res)

	res = c.RegisterRunner(otherToken, "test", "tags")
	assert.Nil(t, res)

	res = c.RegisterRunner(brokenCredentials, "test", "tags")
	assert.Nil(t, res)
}

func TestDeleteRunner(t *testing.T) {
	handler := func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/ci/api/v1/runners/delete" {
			w.WriteHeader(404)
			return
		}

		if r.Method != "DELETE" {
			w.WriteHeader(406)
			return
		}

		body, err := ioutil.ReadAll(r.Body)
		assert.NoError(t, err)

		var req map[string]interface{}
		err = json.Unmarshal(body, &req)
		assert.NoError(t, err)

		switch req["token"].(string) {
		case "valid":
			w.WriteHeader(200)
		case "invalid":
			w.WriteHeader(403)
		default:
			w.WriteHeader(400)
		}
	}

	s := httptest.NewServer(http.HandlerFunc(handler))
	defer s.Close()

	validToken := RunnerCredentials{
		URL:   s.URL,
		Token: "valid",
	}

	invalidToken := RunnerCredentials{
		URL:   s.URL,
		Token: "invalid",
	}

	otherToken := RunnerCredentials{
		URL:   s.URL,
		Token: "other",
	}

	c := GitLabClient{}

	state := c.DeleteRunner(validToken)
	assert.True(t, state)

	state = c.DeleteRunner(invalidToken)
	assert.False(t, state)

	state = c.DeleteRunner(otherToken)
	assert.False(t, state)

	state = c.DeleteRunner(brokenCredentials)
	assert.False(t, state)
}

func TestVerifyRunner(t *testing.T) {
	handler := func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/ci/api/v1/builds/-1" {
			w.WriteHeader(404)
			return
		}

		if r.Method != "PUT" {
			w.WriteHeader(406)
			return
		}

		body, err := ioutil.ReadAll(r.Body)
		assert.NoError(t, err)

		var req map[string]interface{}
		err = json.Unmarshal(body, &req)
		assert.NoError(t, err)

		switch req["token"].(string) {
		case "valid":
			w.WriteHeader(404) // since the build id is broken, we should not find this build
		case "invalid":
			w.WriteHeader(403)
		default:
			w.WriteHeader(400)
		}
	}

	s := httptest.NewServer(http.HandlerFunc(handler))
	defer s.Close()

	validToken := RunnerCredentials{
		URL:   s.URL,
		Token: "valid",
	}

	invalidToken := RunnerCredentials{
		URL:   s.URL,
		Token: "invalid",
	}

	otherToken := RunnerCredentials{
		URL:   s.URL,
		Token: "other",
	}

	c := GitLabClient{}

	state := c.VerifyRunner(validToken)
	assert.True(t, state)

	state = c.VerifyRunner(invalidToken)
	assert.False(t, state)

	state = c.VerifyRunner(otherToken)
	assert.True(t, state, "in other cases where we can't explicitly say that runner is valid we say that it's")

	state = c.VerifyRunner(brokenCredentials)
	assert.False(t, state)
}

func TestUpdateBuild(t *testing.T) {
	handler := func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/ci/api/v1/builds/10.json" {
			w.WriteHeader(404)
			return
		}

		if r.Method != "PUT" {
			w.WriteHeader(406)
			return
		}

		body, err := ioutil.ReadAll(r.Body)
		assert.NoError(t, err)

		var req map[string]interface{}
		err = json.Unmarshal(body, &req)
		assert.NoError(t, err)

		assert.Equal(t, "token", req["token"])
		assert.Equal(t, "trace", req["trace"])

		switch req["state"].(string) {
		case "running":
			w.WriteHeader(200)
		case "forbidden":
			w.WriteHeader(403)
		default:
			w.WriteHeader(400)
		}
	}

	s := httptest.NewServer(http.HandlerFunc(handler))
	defer s.Close()

	config := RunnerConfig{
		RunnerCredentials: RunnerCredentials{
			URL:   s.URL,
			Token: "token",
		},
	}

	trace := "trace"
	c := GitLabClient{}

	state := c.UpdateBuild(config, 10, "running", &trace)
	assert.Equal(t, UpdateSucceeded, state, "Update should continue when running")

	state = c.UpdateBuild(config, 10, "forbidden", &trace)
	assert.Equal(t, UpdateAbort, state, "Update should if the state is forbidden")

	state = c.UpdateBuild(config, 10, "other", &trace)
	assert.Equal(t, UpdateFailed, state, "Update should fail for badly formatted request")

	state = c.UpdateBuild(config, 4, "state", &trace)
	assert.Equal(t, UpdateAbort, state, "Update should abort for unknown build")

	state = c.UpdateBuild(brokenConfig, 4, "state", &trace)
	assert.Equal(t, UpdateAbort, state)
}

func TestArtifactsUpload(t *testing.T) {
	handler := func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/ci/api/v1/builds/10/artifacts" {
			w.WriteHeader(404)
			return
		}

		if r.Method != "POST" {
			w.WriteHeader(406)
			return
		}

		if r.Header.Get("BUILD-TOKEN") != "token" {
			w.WriteHeader(403)
			return
		}

		file, _, err := r.FormFile("file")
		if err != nil {
			w.WriteHeader(400)
			return
		}

		body, err := ioutil.ReadAll(file)
		assert.NoError(t, err)

		if string(body) != "content" {
			w.WriteHeader(413)
		} else {
			w.WriteHeader(201)
		}
	}

	s := httptest.NewServer(http.HandlerFunc(handler))
	defer s.Close()

	config := BuildCredentials{
		ID:    10,
		URL:   s.URL,
		Token: "token",
	}
	invalidToken := BuildCredentials{
		ID:    10,
		URL:   s.URL,
		Token: "invalid-token",
	}

	tempFile, err := ioutil.TempFile("", "artifacts")
	assert.NoError(t, err)
	defer tempFile.Close()
	defer os.Remove(tempFile.Name())

	c := GitLabClient{}

	fmt.Fprint(tempFile, "content")
	state := c.UploadArtifacts(config, tempFile.Name())
	assert.Equal(t, UploadSucceeded, state, "Artifacts should be uploaded")

	fmt.Fprint(tempFile, "too large")
	state = c.UploadArtifacts(config, tempFile.Name())
	assert.Equal(t, UploadTooLarge, state, "Artifacts should be not uploaded, because of too large archive")

	state = c.UploadArtifacts(config, "not/existing/file")
	assert.Equal(t, UploadFailed, state, "Artifacts should fail to be uploaded")

	state = c.UploadArtifacts(invalidToken, tempFile.Name())
	assert.Equal(t, UploadForbidden, state, "Artifacts should be rejected if invalid token")
}

var patchToken = "token"
var patchTraceString = "trace trace trace"

func getPatchServer(t *testing.T, handler func(w http.ResponseWriter, r *http.Request, body string, offset, limit int)) (*httptest.Server, GitLabClient, RunnerConfig) {
	patchHandler := func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "PATCH" {
			w.WriteHeader(406)
			return
		}

		assert.Equal(t, patchToken, r.Header.Get("BUILD-TOKEN"))

		body, err := ioutil.ReadAll(r.Body)
		assert.NoError(t, err)

		contentRange := r.Header.Get("Content-Range")
		ranges := strings.Split(contentRange, "-")

		offset, err := strconv.Atoi(ranges[0])
		assert.NoError(t, err)

		limit, err := strconv.Atoi(ranges[1])
		assert.NoError(t, err)

		handler(w, r, string(body), offset, limit)
	}

	server := httptest.NewServer(http.HandlerFunc(patchHandler))

	config := RunnerConfig{
		RunnerCredentials: RunnerCredentials{
			URL: server.URL,
		},
	}

	return server, GitLabClient{}, config
}

func getTracePatch(traceString string, offset int) *tracePatch {
	trace := bytes.Buffer{}
	trace.WriteString(traceString)
	tracePatch, _ := newTracePatch(trace, offset)

	return tracePatch
}

func TestUnknownPatchTrace(t *testing.T) {
	handler := func(w http.ResponseWriter, r *http.Request, body string, offset, limit int) {
		w.WriteHeader(404)
	}

	server, client, config := getPatchServer(t, handler)
	defer server.Close()

	tracePatch := getTracePatch(patchTraceString, 0)
	state := client.PatchTrace(config, &BuildCredentials{ID: 1, Token: patchToken}, tracePatch)
	assert.Equal(t, UpdateNotFound, state)
}

func TestForbiddenPatchTrace(t *testing.T) {
	handler := func(w http.ResponseWriter, r *http.Request, body string, offset, limit int) {
		w.WriteHeader(403)
	}

	server, client, config := getPatchServer(t, handler)
	defer server.Close()

	tracePatch := getTracePatch(patchTraceString, 0)
	state := client.PatchTrace(config, &BuildCredentials{ID: 1, Token: patchToken}, tracePatch)
	assert.Equal(t, UpdateAbort, state)
}

func TestPatchTrace(t *testing.T) {
	handler := func(w http.ResponseWriter, r *http.Request, body string, offset, limit int) {
		assert.Equal(t, patchTraceString[offset:limit], body)

		w.WriteHeader(202)
	}

	server, client, config := getPatchServer(t, handler)
	defer server.Close()

	tracePatch := getTracePatch(patchTraceString, 0)
	state := client.PatchTrace(config, &BuildCredentials{ID: 1, Token: patchToken}, tracePatch)
	assert.Equal(t, UpdateSucceeded, state)

	tracePatch = getTracePatch(patchTraceString, 3)
	state = client.PatchTrace(config, &BuildCredentials{ID: 1, Token: patchToken}, tracePatch)
	assert.Equal(t, UpdateSucceeded, state)

	tracePatch = getTracePatch(patchTraceString[:10], 3)
	state = client.PatchTrace(config, &BuildCredentials{ID: 1, Token: patchToken}, tracePatch)
	assert.Equal(t, UpdateSucceeded, state)
}

func TestRangeMismatchPatchTrace(t *testing.T) {
	handler := func(w http.ResponseWriter, r *http.Request, body string, offset, limit int) {
		if offset > 10 {
			w.Header().Set("Range", "0-10")
			w.WriteHeader(416)
		}

		w.WriteHeader(202)
	}

	server, client, config := getPatchServer(t, handler)
	defer server.Close()

	tracePatch := getTracePatch(patchTraceString, 11)
	state := client.PatchTrace(config, &BuildCredentials{ID: 1, Token: patchToken}, tracePatch)
	assert.Equal(t, UpdateRangeMismatch, state)

	tracePatch = getTracePatch(patchTraceString, 15)
	state = client.PatchTrace(config, &BuildCredentials{ID: 1, Token: patchToken}, tracePatch)
	assert.Equal(t, UpdateRangeMismatch, state)

	tracePatch = getTracePatch(patchTraceString, 5)
	state = client.PatchTrace(config, &BuildCredentials{ID: 1, Token: patchToken}, tracePatch)
	assert.Equal(t, UpdateSucceeded, state)
}

func TestResendPatchTrace(t *testing.T) {
	handler := func(w http.ResponseWriter, r *http.Request, body string, offset, limit int) {
		if offset > 10 {
			w.Header().Set("Range", "0-10")
			w.WriteHeader(416)
		}

		w.WriteHeader(202)
	}

	server, client, config := getPatchServer(t, handler)
	defer server.Close()

	tracePatch := getTracePatch(patchTraceString, 11)
	state := client.PatchTrace(config, &BuildCredentials{ID: 1, Token: patchToken}, tracePatch)
	assert.Equal(t, UpdateRangeMismatch, state)

	state = client.PatchTrace(config, &BuildCredentials{ID: 1, Token: patchToken}, tracePatch)
	assert.Equal(t, UpdateSucceeded, state)
}

// We've had a situation where the same build was triggered second time by GItLab. In GitLab the build trace
// was 17041 bytes long while the repeated build trace was only 66 bytes long. We've had a `RangeMismatch`
// response, so the offset was updated (to 17041) and `client.PatchTrace` was repeated, at it was planned.
// Unfortunately the `tracePatch` struct was  not resistant to a situation when the offset is set to a
// value bigger than trace's length. This test simulates such situation.
func TestResendDoubledBuildPatchTrace(t *testing.T) {
	handler := func(w http.ResponseWriter, r *http.Request, body string, offset, limit int) {
		if offset > 10 {
			w.Header().Set("Range", "0-100")
			w.WriteHeader(416)
		}

		w.WriteHeader(202)
	}

	server, client, config := getPatchServer(t, handler)
	defer server.Close()

	tracePatch := getTracePatch(patchTraceString, 11)
	state := client.PatchTrace(config, &BuildCredentials{ID: 1, Token: patchToken}, tracePatch)
	assert.Equal(t, UpdateRangeMismatch, state)
	assert.False(t, tracePatch.ValidateRange())
}
