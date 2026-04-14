# Architecture Document: Issue #8 - Add --version flag to CLI

## Problem Statement

The terraprism CLI currently has version functionality implemented in `runVersionMode()`, but the current behavior includes additional information beyond just the version output (terraform/tofu version and update checks). The issue requests a clean, simple version flag that:

- Prints only `terraprism vX.Y.Z` format
- Supports both `--version` and `-v` shorthand  
- Uses version injected at build time via `-ldflags`
- Exits with code 0

The current implementation in lines 76-78 and 638-658 of `cmd/terraprism/main.go` already handles the flags but outputs additional information that may not align with the acceptance criteria.

## Affected Components

| Component | File Path | Impact |
|-----------|-----------|---------|
| Main CLI | `cmd/terraprism/main.go` | Modify `runVersionMode()` function |
| Build System | `Makefile` | Ensure `-ldflags` version injection |

## Solution Design

### Current State Analysis
- Version constant exists at line 20: `const version = "0.11.0"`
- Flag handling exists at lines 76-78 for `-v` and `--version`
- `runVersionMode()` function at lines 638-658 currently:
  - Prints `terraprism v{version}` ✓
  - Also prints terraform/tofu version info ❌ (not requested)
  - Checks for updates ❌ (not requested)

### Proposed Changes

#### Option A: Minimal Version Output (Recommended)
Create a separate simple version function that only outputs the version string and exits.

#### Option B: Preserve Current Behavior with New Flag
Keep current `runVersionMode()` for `version` command, add simple version for `--version`/`-v`.

**Recommendation: Option A** - Modify existing `runVersionMode()` to provide clean output for CLI flags, keeping it simple and focused.

### Implementation Approach

1. **Simplify `runVersionMode()`**: Remove terraform version check and update notification
2. **Create detailed version command**: If needed, add a separate `terraprism version --detail` or similar
3. **Verify build-time injection**: Ensure Makefile correctly injects version via `-ldflags`

## Implementation Steps

1. **Update `runVersionMode()` function** - Simplify to only print terraprism version
2. **Verify flag handling** - Ensure `-v` and `--version` work correctly  
3. **Check build system** - Verify `-ldflags` version injection in Makefile
4. **Add tests** - Create test for version output format
5. **Update documentation** - Ensure usage text reflects correct behavior

## File Changes

| File | Type | Description |
|------|------|-------------|
| `cmd/terraprism/main.go` | Modify | Simplify `runVersionMode()` function (lines 638-658) |
| `cmd/terraprism/main_test.go` | Create | Add tests for version flag functionality |
| `Makefile` | Verify | Ensure `-ldflags` version injection is present |

## Testing Strategy

### Unit Tests
- Test `--version` flag outputs correct format
- Test `-v` shorthand works identically  
- Test exit code is 0
- Test version string format matches `terraprism vX.Y.Z`

### Integration Tests
- Verify build-time version injection works
- Test version flag with various argument combinations
- Ensure no additional output (terraform version, update checks)

### Manual Testing
```bash
# Expected outputs
./terraprism --version  # Should output: terraprism v0.11.0
./terraprism -v         # Should output: terraprism v0.11.0
echo $?                 # Should output: 0
```

## Risk Assessment

**Low Risk** - This is a modification to existing, working functionality with clear requirements.

### Potential Issues
- Breaking change if users rely on current verbose version output
- Build system may need verification for `-ldflags` injection

### Mitigation
- Check if any automation depends on current version output format
- Verify Makefile has proper version injection before implementing
- Consider keeping detailed version info in separate command if needed