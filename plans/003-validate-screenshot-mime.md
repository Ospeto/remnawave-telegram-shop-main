# Plan 003: Validate screenshot bytes before vision analysis

> **Executor instructions**: Follow this plan step by step. Run every verification command and confirm the expected result before moving to the next step. If anything in the "STOP conditions" section occurs, stop and report - do not improvise. When done, update the status row for this plan in `plans/README.md` unless a reviewer dispatched you and told you they maintain the index.
>
> **Drift check (run first)**: `git diff --stat 8e80b0b..HEAD -- internal/api/handlers.go internal/api/*_test.go internal/gemini/openrouter.go internal/gemini/gemini.go`
> If any in-scope file changed since this plan was written, compare the "Current state" excerpts against the live code before proceeding; on a mismatch, treat it as a STOP condition.

## Status

- **Priority**: P1
- **Effort**: S-M
- **Risk**: LOW-MED
- **Depends on**: none
- **Category**: security
- **Planned at**: commit `8e80b0b`, 2026-06-20

## Why This Matters

Screenshot verification sends uploaded bytes to a model provider. The handler limits the body size, but it trusts the multipart `Content-Type` header when present. A client can label non-image content as an image and cause unnecessary provider calls or confusing verification failures. The backend should validate the actual bytes and pass a trusted MIME type to Gemini/OpenRouter.

## Current State

- `internal/api/handlers.go` - upload handler reads multipart file and forwards MIME type to payment verification.
- `internal/gemini/openrouter.go` - builds a `data:<mime>;base64,...` URL for OpenRouter.
- `internal/gemini/gemini.go` - sends `InlineData.MimeType` to Gemini.

Current excerpts:

```go
// internal/api/handlers.go:1665
// Now safe to read the file. Limit body to 10 MB.
r.Body = http.MaxBytesReader(w, r.Body, 10<<20)
if err := r.ParseMultipartForm(10 << 20); err != nil {
    ...
}
```

```go
// internal/api/handlers.go:1706
mimeType := header.Header.Get("Content-Type")
if mimeType == "" {
    mimeType = http.DetectContentType(fileBytes)
}

result, err := h.paymentService.VerifyMobilePayment(r.Context(), int64(purchaseID), fileBytes, mimeType)
```

```go
// internal/gemini/openrouter.go:106
func (c *OpenRouterClient) AnalyzePaymentScreenshot(ctx context.Context, imageBytes []byte, mimeType string, providers []ConfiguredProvider) (*PaymentInfo, error) {
    imageData := fmt.Sprintf("data:%s;base64,%s", mimeType, base64.StdEncoding.EncodeToString(imageBytes))
```

```go
// internal/gemini/gemini.go:260
func (c *Client) AnalyzePaymentScreenshot(ctx context.Context, imageBytes []byte, mimeType string, providers []ConfiguredProvider) (*PaymentInfo, error) {
    b64Image := base64.StdEncoding.EncodeToString(imageBytes)
    ...
    MimeType: mimeType,
```

Repo conventions:

- Upload rejection logging uses named reasons such as `uploadRejectInvalidMultipartForm` near `internal/api/handlers.go:575`.
- API tests use `httptest` and focused helper tests where full dependency wiring would be heavy.

## Commands You Will Need

| Purpose | Command | Expected on success |
|---------|---------|---------------------|
| API tests | `go test ./internal/api` | exit 0 |
| Gemini tests | `go test ./internal/gemini` | exit 0 |
| Full Go tests | `go test ./...` | exit 0 |
| Vet | `go vet ./...` | exit 0 |

## Scope

**In scope**:
- `internal/api/handlers.go`
- Existing or new `internal/api/*_test.go`

**Out of scope**:
- Do not change Gemini/OpenRouter request formats unless tests show they are already wrong.
- Do not change max upload size in this plan.
- Do not add image transcoding or metadata stripping.
- Do not support ambiguous formats like HEIC by guessing. If HEIC support is a product requirement, stop and ask for explicit acceptance criteria.

## Git Workflow

- Branch: `codex/003-validate-screenshot-mime`
- Commit message: `fix: validate payment screenshot mime type`
- Do not push or open a PR unless instructed.

## Steps

### Step 1: Add a trusted screenshot MIME helper

In `internal/api/handlers.go`, add an unexported helper close to upload-related code:

```go
func trustedPaymentScreenshotMIME(fileBytes []byte) (string, bool) {
    detected := http.DetectContentType(fileBytes)
    switch detected {
    case "image/jpeg", "image/png", "image/webp", "image/gif":
        return detected, true
    default:
        return detected, false
    }
}
```

If the project already has MIME helpers in the live code, extend that instead of adding a second helper.

**Verify**: `go test ./internal/api` -> exit 0 or only fails for missing tests you are about to add.

### Step 2: Use detected MIME instead of the client header

Replace the current `header.Header.Get("Content-Type")` trust path with byte detection:

```go
mimeType, ok := trustedPaymentScreenshotMIME(fileBytes)
if !ok {
    logUploadScreenshotReject(..., uploadRejectUnsupportedFileType, http.StatusBadRequest, nil, ...)
    http.Error(w, "Unsupported file type", http.StatusBadRequest)
    return
}
```

Add a new rejection reason constant such as `uploadRejectUnsupportedFileType`. Keep existing logging fields like `purchase_id`, `invoice_type`, `mime_type`, and `file_size`; use the detected MIME type, not the client header.

Then call:

```go
h.paymentService.VerifyMobilePayment(..., fileBytes, mimeType)
```

**Verify**: `go test ./internal/api` -> exit 0.

### Step 3: Add helper tests for allowed and rejected bytes

Add table-driven tests in an API test file for `trustedPaymentScreenshotMIME`:

- Minimal PNG signature returns `image/png`, true.
- Minimal JPEG signature returns `image/jpeg`, true.
- Plain text returns `text/plain; charset=utf-8`, false.
- Empty or tiny invalid bytes return false.

Use byte slices; do not require real image fixtures.

**Verify**: `go test ./internal/api` -> exit 0 and the tests fail if the handler trusts client headers again.

### Step 4: Run the provider-facing package tests

The provider code should not need changes, but run its tests to ensure no expected MIME behavior was broken.

**Verify**: `go test ./internal/gemini` -> exit 0.

## Test Plan

- New helper tests for MIME detection and rejection.
- Existing upload handler tests should continue passing.
- Full verification: `go test ./...`, `go vet ./...`, `go build ./cmd/app`.

## Done Criteria

- [ ] The upload handler no longer uses multipart `Content-Type` as the source of truth.
- [ ] Unsupported bytes return HTTP 400 before model-provider analysis.
- [ ] Accepted files pass the detected MIME type to `VerifyMobilePayment`.
- [ ] `go test ./internal/api` exits 0.
- [ ] `go test ./...` exits 0.
- [ ] `go vet ./...` exits 0.
- [ ] `plans/README.md` status row updated.

## STOP Conditions

Stop and report back if:

- Existing production requirements explicitly need file types `http.DetectContentType` cannot identify.
- The live upload handler has been refactored enough that the current-state excerpt no longer applies.
- The fix requires changing model-provider API contracts.

## Maintenance Notes

If new payment screenshot formats are allowed later, add a test case first and include the provider behavior in review. Do not expand the allowlist with broad prefixes like `strings.HasPrefix(mime, "image/")` without a cost/security review.
