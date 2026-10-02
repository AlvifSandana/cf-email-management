package cloudflare

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestClient_ListWorkers(t *testing.T) {
	t.Run("Success", func(t *testing.T) {
		createdTime, _ := time.Parse(time.RFC3339, "2026-01-10T12:00:00Z")
		modifiedTime, _ := time.Parse(time.RFC3339, "2026-01-12T15:30:00Z")

		mux := http.NewServeMux()
		mux.HandleFunc("GET /accounts/acc_123/workers/scripts", func(w http.ResponseWriter, r *http.Request) {
			if auth := r.Header.Get("Authorization"); auth != "Bearer test_token" {
				t.Errorf("expected Authorization header 'Bearer test_token', got '%s'", auth)
			}
			if accept := r.Header.Get("Accept"); accept != "application/json" {
				t.Errorf("expected Accept header 'application/json', got '%s'", accept)
			}

			resp := cfResponse[[]WorkerSummary]{
				Success: true,
				Result: []WorkerSummary{
					{
						ID:         "email-handler",
						ETag:       "etag_abc123",
						CreatedOn:  createdTime,
						ModifiedOn: modifiedTime,
					},
					{
						ID:         "filter-worker",
						ETag:       "etag_xyz789",
						CreatedOn:  createdTime,
						ModifiedOn: modifiedTime,
					},
				},
			}
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(resp)
		})

		server := httptest.NewServer(mux)
		defer server.Close()

		client := NewClient("test_token", WithBaseURL(server.URL))
		workers, err := client.ListWorkers(context.Background(), "acc_123")
		if err != nil {
			t.Fatalf("ListWorkers failed: %v", err)
		}

		if len(workers) != 2 {
			t.Fatalf("expected 2 workers, got %d", len(workers))
		}

		if workers[0].ID != "email-handler" || workers[0].ETag != "etag_abc123" {
			t.Errorf("worker[0] mismatch: %+v", workers[0])
		}
		if !workers[0].CreatedOn.Equal(createdTime) || !workers[0].ModifiedOn.Equal(modifiedTime) {
			t.Errorf("worker[0] timestamps mismatch: created=%v, modified=%v", workers[0].CreatedOn, workers[0].ModifiedOn)
		}
		if workers[1].ID != "filter-worker" || workers[1].ETag != "etag_xyz789" {
			t.Errorf("worker[1] mismatch: %+v", workers[1])
		}
	})

	t.Run("EmptyList", func(t *testing.T) {
		mux := http.NewServeMux()
		mux.HandleFunc("GET /accounts/acc_empty/workers/scripts", func(w http.ResponseWriter, r *http.Request) {
			resp := cfResponse[[]WorkerSummary]{
				Success: true,
				Result:  []WorkerSummary{},
			}
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(resp)
		})

		server := httptest.NewServer(mux)
		defer server.Close()

		client := NewClient("test_token", WithBaseURL(server.URL))
		workers, err := client.ListWorkers(context.Background(), "acc_empty")
		if err != nil {
			t.Fatalf("ListWorkers failed: %v", err)
		}
		if len(workers) != 0 {
			t.Errorf("expected 0 workers, got %d", len(workers))
		}
	})

	t.Run("APIError", func(t *testing.T) {
		mux := http.NewServeMux()
		mux.HandleFunc("GET /accounts/acc_err/workers/scripts", func(w http.ResponseWriter, r *http.Request) {
			resp := cfResponse[[]WorkerSummary]{
				Success: false,
				Errors: []cfError{
					{Code: 10007, Message: "account not found"},
				},
			}
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(resp)
		})

		server := httptest.NewServer(mux)
		defer server.Close()

		client := NewClient("test_token", WithBaseURL(server.URL))
		_, err := client.ListWorkers(context.Background(), "acc_err")
		if err == nil {
			t.Fatal("expected error from API failure, got nil")
		}
	})

	t.Run("HTTPError", func(t *testing.T) {
		mux := http.NewServeMux()
		mux.HandleFunc("GET /accounts/acc_http_err/workers/scripts", func(w http.ResponseWriter, r *http.Request) {
			http.Error(w, `{"success":false,"errors":[{"code":500,"message":"internal error"}]}`, http.StatusInternalServerError)
		})

		server := httptest.NewServer(mux)
		defer server.Close()

		client := NewClient("test_token", WithBaseURL(server.URL))
		_, err := client.ListWorkers(context.Background(), "acc_http_err")
		if err == nil {
			t.Fatal("expected error from HTTP 500, got nil")
		}
	})
}

func TestClient_UploadWorker(t *testing.T) {
	scriptContent := `export default {
		async email(message, env, ctx) {
			console.log("Email received from: " + message.from);
		}
	};`

	t.Run("Success", func(t *testing.T) {
		var receivedBody string
		var receivedContentType string
		var receivedAuth string
		var receivedAccept string

		mux := http.NewServeMux()
		mux.HandleFunc("PUT /accounts/acc_123/workers/scripts/email-handler", func(w http.ResponseWriter, r *http.Request) {
			receivedAuth = r.Header.Get("Authorization")
			receivedContentType = r.Header.Get("Content-Type")
			receivedAccept = r.Header.Get("Accept")

			bodyBytes, err := io.ReadAll(r.Body)
			if err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
			receivedBody = string(bodyBytes)

			resp := cfResponse[map[string]any]{
				Success: true,
				Result: map[string]any{
					"id":   "email-handler",
					"etag": "etag_new123",
					"size": len(receivedBody),
				},
			}
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(resp)
		})

		server := httptest.NewServer(mux)
		defer server.Close()

		client := NewClient("test_token", WithBaseURL(server.URL))
		err := client.UploadWorker(context.Background(), "acc_123", "email-handler", scriptContent)
		if err != nil {
			t.Fatalf("UploadWorker failed: %v", err)
		}

		if receivedAuth != "Bearer test_token" {
			t.Errorf("expected Authorization 'Bearer test_token', got '%s'", receivedAuth)
		}
		if receivedContentType != "application/javascript" {
			t.Errorf("expected Content-Type 'application/javascript', got '%s'", receivedContentType)
		}
		if receivedAccept != "application/json" {
			t.Errorf("expected Accept 'application/json', got '%s'", receivedAccept)
		}
		if receivedBody != scriptContent {
			t.Errorf("expected script content '%s', got '%s'", scriptContent, receivedBody)
		}
	})

	t.Run("APIError", func(t *testing.T) {
		mux := http.NewServeMux()
		mux.HandleFunc("PUT /accounts/acc_err/workers/scripts/invalid-script", func(w http.ResponseWriter, r *http.Request) {
			resp := cfResponse[any]{
				Success: false,
				Errors: []cfError{
					{Code: 10021, Message: "Syntax error in worker script"},
				},
			}
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(resp)
		})

		server := httptest.NewServer(mux)
		defer server.Close()

		client := NewClient("test_token", WithBaseURL(server.URL))
		err := client.UploadWorker(context.Background(), "acc_err", "invalid-script", "bad script")
		if err == nil {
			t.Fatal("expected error from API failure, got nil")
		}
	})

	t.Run("HTTPError", func(t *testing.T) {
		mux := http.NewServeMux()
		mux.HandleFunc("PUT /accounts/acc_err/workers/scripts/bad-req", func(w http.ResponseWriter, r *http.Request) {
			http.Error(w, `{"success":false,"errors":[{"code":400,"message":"bad request"}]}`, http.StatusBadRequest)
		})

		server := httptest.NewServer(mux)
		defer server.Close()

		client := NewClient("test_token", WithBaseURL(server.URL))
		err := client.UploadWorker(context.Background(), "acc_err", "bad-req", "const a = 1;")
		if err == nil {
			t.Fatal("expected error from HTTP 400, got nil")
		}
	})
}

func TestClient_DeleteWorker(t *testing.T) {
	t.Run("Success", func(t *testing.T) {
		var receivedAuth string
		mux := http.NewServeMux()
		mux.HandleFunc("DELETE /accounts/acc_123/workers/scripts/email-handler", func(w http.ResponseWriter, r *http.Request) {
			receivedAuth = r.Header.Get("Authorization")
			resp := cfResponse[map[string]any]{
				Success: true,
				Result: map[string]any{
					"id": "email-handler",
				},
			}
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(resp)
		})

		server := httptest.NewServer(mux)
		defer server.Close()

		client := NewClient("test_token", WithBaseURL(server.URL))
		err := client.DeleteWorker(context.Background(), "acc_123", "email-handler")
		if err != nil {
			t.Fatalf("DeleteWorker failed: %v", err)
		}

		if receivedAuth != "Bearer test_token" {
			t.Errorf("expected Authorization 'Bearer test_token', got '%s'", receivedAuth)
		}
	})

	t.Run("APIError", func(t *testing.T) {
		mux := http.NewServeMux()
		mux.HandleFunc("DELETE /accounts/acc_err/workers/scripts/not-found", func(w http.ResponseWriter, r *http.Request) {
			resp := cfResponse[any]{
				Success: false,
				Errors: []cfError{
					{Code: 10007, Message: "Worker script not found"},
				},
			}
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(resp)
		})

		server := httptest.NewServer(mux)
		defer server.Close()

		client := NewClient("test_token", WithBaseURL(server.URL))
		err := client.DeleteWorker(context.Background(), "acc_err", "not-found")
		if err == nil {
			t.Fatal("expected error from API failure, got nil")
		}
	})

	t.Run("HTTPError", func(t *testing.T) {
		mux := http.NewServeMux()
		mux.HandleFunc("DELETE /accounts/acc_err/workers/scripts/missing", func(w http.ResponseWriter, r *http.Request) {
			http.Error(w, `{"success":false,"errors":[{"code":404,"message":"not found"}]}`, http.StatusNotFound)
		})

		server := httptest.NewServer(mux)
		defer server.Close()

		client := NewClient("test_token", WithBaseURL(server.URL))
		err := client.DeleteWorker(context.Background(), "acc_err", "missing")
		if err == nil {
			t.Fatal("expected error from HTTP 404, got nil")
		}
	})
}
