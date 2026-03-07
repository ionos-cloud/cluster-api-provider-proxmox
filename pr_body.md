## What changed

Replaced all instances of `ctrl.Result{Requeue: true}` with `ctrl.Result{RequeueAfter: 0}` in the ProxmoxCluster controller. Three occurrences were updated in the external managed control plane endpoint validation logic.

## Why

`reconcile.Result{Requeue: true}` is deprecated and will be removed in a future version of controller-runtime. Using `RequeueAfter: 0` achieves the same immediate requeue behavior.

Fixes #652

## Testing

- Project builds successfully (`go build ./...`)
- `go vet ./...` passes with no issues
- All non-envtest tests pass (`go test ./internal/service/... ./pkg/...`)
- Verified no remaining instances of `Requeue: true` in the codebase
