# Cluster API v1beta2 Migration Summary

This document summarizes the migration from cluster-api v1beta1 to v1beta2 API contract.

## Migration Status: ✅ COMPLETE

- **Code compiles:** ✅ Yes
- **Tests pass:** ✅ Yes (all tests passing)
- **CRDs generated:** ✅ Yes
- **Deepcopy generated:** ✅ Yes

## Key Changes

### 1. API Imports Updated

All imports have been updated from v1beta1 to v1beta2:
- `sigs.k8s.io/cluster-api/api/v1beta1` → `sigs.k8s.io/cluster-api/api/core/v1beta2`
- Using non-deprecated `conditions` and `patch` packages

### 2. Conditions API Migration

**v1alpha2 Types:**
- Changed `Conditions` field from `*[]clusterv1.Condition` to `[]metav1.Condition`
- Updated `GetConditions()` to return `[]metav1.Condition`
- Updated `SetConditions()` to accept `[]metav1.Condition`

**Throughout codebase:**
- Replaced `conditions.MarkFalse()` with `conditions.Set()` using `metav1.Condition`
- Replaced `conditions.SetSummary()` with `conditions.SetSummaryCondition()`

### 3. Deprecated Field Access

CAPI core types now access deprecated fields via `Status.Deprecated.V1Beta1.*`:
- `cluster.Status.FailureReason` → `cluster.Status.Deprecated.V1Beta1.FailureReason`
- `cluster.Status.FailureMessage` → `cluster.Status.Deprecated.V1Beta1.FailureMessage`
- `machine.Status.BootstrapReady` → checked via conditions API

### 4. Infrastructure Provider Types

ProxmoxMachine and ProxmoxCluster types maintain their own failure fields as per v1beta2 contract:
- `Status.FailureReason` - kept as-is
- `Status.FailureMessage` - kept as-is
- These are NOT in deprecated structs for infrastructure types

### 5. Conversion Functions

Added proper conversion between v1alpha1 and v1alpha2:
- `ConvertConditionsV1Beta1ToMetav1()` - converts from `[]clusterv1.Condition` to `[]metav1.Condition`
- `ConvertConditionsMetav1ToV1Beta1()` - converts from `[]metav1.Condition` to `[]clusterv1.Condition`

### 6. IPAM API Changes

Updated for v1beta2 IPAM provider changes:
- `ipamutil.ClaimsForCluster()` now takes cluster object directly (not ObjectMeta)
- Test helpers updated accordingly

## Files Modified

### API Types
- `api/v1alpha1/conversion.go` - Added conversion functions
- `api/v1alpha1/zz_generated.conversion.go` - Regenerated
- `api/v1alpha2/conditions_consts.go` - Added new constants
- `api/v1alpha2/proxmoxcluster_types.go` - Updated Conditions field
- `api/v1alpha2/proxmoxmachine_types.go` - Updated Conditions field
- `api/v1alpha2/zz_generated.deepcopy.go` - Regenerated

### Controllers
- `internal/controller/proxmoxcluster_controller.go` - Updated for v1beta2
- `internal/controller/proxmoxcluster_controller_test.go` - Updated tests
- `internal/controller/proxmoxmachine_controller.go` - Updated for v1beta2

### Services
- `internal/service/scheduler/vmscheduler.go` - Updated imports
- `internal/service/taskservice/task.go` - Updated imports
- `internal/service/vmservice/*.go` - Updated all VM service files

### Scope
- `pkg/scope/cluster.go` - Updated patch and conditions API
- `pkg/scope/machine.go` - Updated patch and conditions API
- `pkg/kubernetes/ipam/ipam.go` - Updated for IPAM API changes

### CRDs
- `config/crd/bases/*.yaml` - All CRDs regenerated

### Tests
- `test/e2e/*.go` - Updated e2e tests
- Various `*_test.go` files - Updated unit tests

## Verification

```bash
# Build succeeds
go build ./...

# Tests pass
make test
# Result: All tests passing

# Generation is up to date
make generate manifests
# Result: No changes needed
```

## Compliance with v1beta2 Contract

✅ **Infrastructure Provider Types**: Keep own `failureReason`/`failureMessage` fields  
✅ **CAPI Core Types**: Access deprecated fields via `Status.Deprecated.V1Beta1.*`  
✅ **Conditions**: Use standard `[]metav1.Condition` format  
✅ **Imports**: All using v1beta2 non-deprecated packages  
✅ **API Contract**: Follows [v1.10-to-v1.11 migration guide](https://main.cluster-api.sigs.k8s.io/developer/providers/migrations/v1.10-to-v1.11)

## Next Steps

1. ✅ Code compiles
2. ✅ Tests pass
3. ✅ CRDs regenerated
4. Ready for review and merge

## Migration Reference

Based on the official Cluster API migration guide:
https://main.cluster-api.sigs.k8s.io/developer/providers/migrations/v1.10-to-v1.11
