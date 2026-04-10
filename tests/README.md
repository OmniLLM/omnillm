# UI Testing Guide

This guide covers the comprehensive UI tests for the Provider Management functionality in Open LLM Proxy.

## Overview

The test suite includes two main test files:

1. **`provider-management-ui.test.ts`** - Tests all basic UI operations
2. **`provider-ui-workflows.test.ts`** - Tests realistic user workflows and edge cases

## Test Coverage

### Provider Management Operations
- ✅ Listing providers
- ✅ Creating new provider instances for all types
- ✅ Deleting provider instances
- ✅ Provider activation/deactivation
- ✅ Provider authentication flows
- ✅ Model management (listing, toggling)
- ✅ Usage data retrieval
- ✅ Priority management for multiple providers

### User Workflows
- ✅ Complete provider lifecycle (create → auth → activate → deactivate → delete)
- ✅ Multi-provider scenarios
- ✅ Error recovery scenarios
- ✅ Concurrent operations
- ✅ Data consistency checks

### Edge Cases
- ✅ Empty provider lists
- ✅ Malformed requests
- ✅ Concurrent auth requests
- ✅ Provider cleanup verification

### API Endpoints Tested
- ✅ `GET /api/admin/providers` - List providers
- ✅ `POST /api/admin/providers/{type}/add-instance` - Create instances
- ✅ `DELETE /api/admin/providers/{id}` - Delete instances
- ✅ `POST /api/admin/providers/{id}/activate` - Activate providers
- ✅ `POST /api/admin/providers/{id}/deactivate` - Deactivate providers
- ✅ `POST /api/admin/providers/{id}/auth` - Authenticate providers
- ✅ `GET /api/admin/providers/{id}/models` - List models
- ✅ `POST /api/admin/providers/{id}/models/toggle` - Toggle models
- ✅ `GET /api/admin/providers/{id}/usage` - Get usage data
- ✅ `GET /api/admin/providers/priorities` - Get priorities
- ✅ `POST /api/admin/providers/priorities` - Set priorities
- ✅ `GET /api/admin/status` - Server status
- ✅ `GET /api/admin/info` - Server info
- ✅ `GET /api/admin/auth-status` - Auth flow status
- ✅ `GET /models` - Models endpoint
- ✅ `POST /chat/completions` - Chat completions endpoint

## How to Run Tests

### Prerequisites

No external server is required for the UI suites. Each test file starts its own
isolated proxy server with a temporary `HOME`/`USERPROFILE`, so it does not
touch your real provider database or tokens.

### Running Tests

Dangerous suites:
`integration.test.ts`, `provider-management-ui.test.ts`, `provider-ui-workflows.test.ts`, and `material-ui-visual.test.ts` are skipped by default because they can trigger live provider work or expensive local integration flows. Run them only when intentionally requested by setting `OMNIMODEL_RUN_DANGEROUS_TESTS=1`.

PowerShell example:
```powershell
$env:OMNIMODEL_RUN_DANGEROUS_TESTS = "1"
bun test tests/integration.test.ts
```

#### Option 1: Use the test runner script (Recommended)
```bash
# Run all UI tests
./run-ui-tests.sh

# Run only basic provider tests  
./run-ui-tests.sh --basic

# Run only workflow tests
./run-ui-tests.sh --workflows
```

#### Option 2: Use npm/bun scripts
```bash
# Run all UI tests
bun run test:ui

# Run individual test files
bun test tests/provider-management-ui.test.ts
bun test tests/provider-ui-workflows.test.ts

# Run all tests
bun run test
```

#### Option 3: Run tests directly
```bash
bun test tests/provider-management-ui.test.ts --reporter verbose
bun test tests/provider-ui-workflows.test.ts --reporter verbose
```

### Test Configuration

Tests are configured to:
- Start an isolated proxy server automatically
- Store config, database, tokens, and logs in a temporary directory
- Handle authentication failures gracefully (expected with test data)
- Test error scenarios and edge cases

## Expected Test Results

### Successful Operations
- Provider CRUD operations should work
- Activation/deactivation should update state
- Model toggles should be recorded
- Priority setting should persist

### Expected Failures (These are normal)
- Authentication with test credentials will fail (expected)
- Model fetching from unauthenticated providers will fail (expected)
- Usage data from unauthenticated providers will fail (expected)

### Error Handling
- Invalid provider types should return 400 errors
- Missing authentication data should return 400 errors
- Operations on non-existent providers should return 404 errors

## Test Data Cleanup

The tests automatically remove their temporary server sandbox after each test, including:
- Provider instances
- Modified priority settings
- Database files
- Token files
- Log files

## Troubleshooting

### "Server not running" error
- The isolated test server may have failed to start
- Re-run the failing test and inspect the startup error output

### Test timeouts
- The server may be slow to start or process requests
- Increase timeout values in test configuration if needed

### Authentication test failures
- This is expected behavior with test credentials
- Tests verify the request/response structure, not actual auth

### Persistent test data
- UI tests should not leave persistent provider data behind
- If you still see leaked providers, they came from older test runs against a live server

## Adding New Tests

When adding new UI functionality, add corresponding tests:

1. **Basic operations** → Add to `provider-management-ui.test.ts`
2. **User workflows** → Add to `provider-ui-workflows.test.ts`
3. **Follow the existing patterns** for API calls and isolated server startup
4. **Test both success and error cases**
5. **Keep each test isolated so it can run against a fresh temporary server**

## Performance Testing

The tests also serve as basic performance tests:
- Measure response times for key operations
- Test concurrent request handling
- Verify memory usage during batch operations

For detailed performance testing, run:
```bash
bun test --reporter verbose --timeout 30000
```

This gives detailed timing information for each test case.
