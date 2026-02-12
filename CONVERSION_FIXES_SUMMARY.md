# ObjectMeta Conversion Functions - Fixed

## Issue
The ObjectMeta conversion functions in `api/v1alpha1/conversion.go` had incorrect type signatures:
- Both `in` and `out` parameters used `clusterv1beta1.ObjectMeta`
- Functions did nothing (just copied input to output of same type)
- They were supposed to convert between v1beta1 and v1beta2 ObjectMeta types

## Root Cause
- v1alpha1 uses `clusterv1 "sigs.k8s.io/cluster-api/api/core/v1beta1"`
- v1alpha2 uses `clusterv1 "sigs.k8s.io/cluster-api/api/core/v1beta2"`
- Template resources (ProxmoxClusterTemplateResource, ProxmoxMachineTemplateResource) have ObjectMeta fields that need conversion between these versions

## Solution
1. **Added v1beta2 import**: 
   ```go
   clusterv1beta2 "sigs.k8s.io/cluster-api/api/core/v1beta2"
   ```

2. **Fixed function signatures**:
   - `Convert_v1beta1_ObjectMeta_To_v1beta2_ObjectMeta(in *clusterv1beta1.ObjectMeta, out *clusterv1beta2.ObjectMeta, ...)`
   - `Convert_v1beta2_ObjectMeta_To_v1beta1_ObjectMeta(in *clusterv1beta2.ObjectMeta, out *clusterv1beta1.ObjectMeta, ...)`

3. **Implemented proper conversion logic**:
   - Copies Labels map (key-by-key)
   - Copies Annotations map (key-by-key)
   - Both v1beta1 and v1beta2 ObjectMeta have identical structure

4. **Updated generated conversion file**:
   - Replaced `compileErrorOnMissingConversion()` calls with actual conversion function calls
   - Fixed 4 auto-generated conversion functions in `zz_generated.conversion.go`

## Verification
- ✅ Code compiles: `go build ./...`
- ✅ No compilation errors
- ✅ All conversion paths now have implementations

## Files Modified
- `api/v1alpha1/conversion.go`: Added import, fixed manual conversion functions
- `api/v1alpha1/zz_generated.conversion.go`: Updated to call conversion functions

## Commit
- 45e0eb4: "Fix ObjectMeta conversion functions between v1beta1 and v1beta2"
