/**
 * Test Configuration for UI Tests
 *
 * Modify these settings to customize test behavior
 */

export const TEST_CONFIG = {
  // Server configuration
  SERVER_PORT:
    process.env.TEST_PORT ? Number.parseInt(process.env.TEST_PORT) : 5000,
  SERVER_HOST: process.env.TEST_HOST || "localhost",

  // Test timeouts (in milliseconds)
  DEFAULT_TIMEOUT: 10000,
  AUTH_TIMEOUT: 15000,
  CLEANUP_TIMEOUT: 5000,

  // Test data
  TEST_DATA: {
    ALIBABA: {
      method: "api-key",
      apiKey: "sk-test123456789abcdef",
      region: "global",
    },

    GITHUB: {
      method: "token",
      token: "ghu_test123456789abcdef",
    },

    ANTIGRAVITY: {
      method: "oauth",
      clientId: "test-client-id-12345",
      clientSecret: "test-client-secret-67890",
    },
  },

  // Test behavior flags
  CLEANUP_ON_FAILURE: true,
  VERBOSE_ERRORS: process.env.NODE_ENV === "development",
  RETRY_FAILED_REQUESTS: false,

  // Rate limiting for tests
  REQUEST_DELAY: 100, // ms between requests
  MAX_CONCURRENT_REQUESTS: 5,
}

export const RUN_DANGEROUS_TESTS =
  process.env.OMNILLM_RUN_DANGEROUS_TESTS === "1"
  || process.env.OMNILLM_RUN_DANGEROUS_TESTS === "true"

export const DANGEROUS_TESTS_ENV_VAR = "OMNILLM_RUN_DANGEROUS_TESTS"

export const formatDangerousTestSkipMessage = (filePath: string) =>
  `Skipping dangerous test suite ${filePath}. Set ${DANGEROUS_TESTS_ENV_VAR}=1 to run it intentionally.`

export const getServerUrl = () =>
  `http://${TEST_CONFIG.SERVER_HOST}:${TEST_CONFIG.SERVER_PORT}`

export const getTestData = (provider: keyof typeof TEST_CONFIG.TEST_DATA) =>
  TEST_CONFIG.TEST_DATA[provider]
