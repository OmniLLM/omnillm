import {
  Add as AddIcon,
  Delete as DeleteIcon,
  Settings as SettingsIcon,
  Warning as WarningIcon,
  Link as LinkIcon,
} from "@mui/icons-material"
// Material UI components
import {
  Box,
  Card,
  CardContent,
  Typography,
  Button,
  Switch,
  FormControlLabel,
  Alert,
  Chip,
  IconButton,
  CircularProgress,
  Paper,
} from "@mui/material"
import { useCallback, useEffect, useState } from "react"

// Import your existing API functions - keeping all the same functionality
import {
  activateProvider,
  deactivateProvider,
  getAuthStatus,
  listProviders,
  getStatus,
  type AuthFlow,
  type Provider,
  type Status,
} from "@/api"
import { getDeviceAuthCopy } from "@/lib/device-auth"

interface MaterialProvidersPageProps {
  showToast: (msg: string, type?: "success" | "error") => void
}

// Convert your existing Spin component to Material UI
function MaterialSpin({ size = 16 }: { size?: number }) {
  return <CircularProgress size={size} />
}

// Convert AuthFlowBanner to Material Design
function MaterialAuthFlowBanner({
  authFlow,
  providers,
}: {
  authFlow: AuthFlow | null | undefined
  providers: Array<Provider>
}) {
  if (
    !authFlow
    || authFlow.status === "complete"
    || authFlow.status === "error"
  )
    return null

  const name =
    providers.find((p) => p.id === authFlow.providerId)?.name
    ?? authFlow.providerId
  const authCopy = getDeviceAuthCopy(authFlow, providers)

  return (
    <Alert
      severity="warning"
      sx={{ mb: 3 }}
      icon={
        authFlow.status === "pending" ?
          <MaterialSpin size={20} />
        : <WarningIcon />
      }
    >
      {authFlow.status === "pending" && (
        <Typography variant="body2">
          Initiating auth flow for {name}…
        </Typography>
      )}
      {authFlow.status === "awaiting_user" && (
        <Box>
          <Typography variant="subtitle2" gutterBottom>
            Authorization Required — {name}
          </Typography>
          {authFlow.userCode && (
            <Box sx={{ mt: 2 }}>
              <Typography
                variant="caption"
                color="text.secondary"
                display="block"
              >
                {authCopy.codeLabel}
              </Typography>
              <Paper
                sx={{
                  p: 1.5,
                  mt: 1,
                  display: "inline-block",
                  bgcolor: "warning.light",
                  color: "warning.contrastText",
                }}
              >
                <Typography
                  variant="h6"
                  component="code"
                  sx={{
                    fontFamily: "monospace",
                    letterSpacing: "0.2em",
                    fontWeight: 700,
                  }}
                >
                  {authFlow.userCode}
                </Typography>
              </Paper>
              {authCopy.codeHint && (
                <Typography
                  variant="caption"
                  color="text.secondary"
                  display="block"
                  sx={{ mt: 1, lineHeight: 1.5 }}
                >
                  {authCopy.codeHint}
                </Typography>
              )}
            </Box>
          )}
          {authFlow.instructionURL && (
            <Box sx={{ mt: 2 }}>
              <Typography
                variant="caption"
                color="text.secondary"
                display="block"
              >
                Authorization URL:
              </Typography>
              <Button
                variant="outlined"
                size="small"
                startIcon={<LinkIcon />}
                href={authFlow.instructionURL}
                target="_blank"
                sx={{ mt: 1 }}
              >
                Open Authorization Page
              </Button>
            </Box>
          )}
        </Box>
      )}
    </Alert>
  )
}

// This is a simplified version - you'll need to port over all your existing components
// Let me first create the basic structure and then we can add all the functionality

export function MaterialProvidersPage({
  showToast,
}: MaterialProvidersPageProps) {
  // Keep all your existing state management
  const [providers, setProviders] = useState<Array<Provider>>([])
  const [loading, setLoading] = useState(true)
  const [authFlow, setAuthFlow] = useState<AuthFlow | null>(null)
  const [status, setStatus] = useState<Status | null>(null)

  // Keep all your existing useEffect hooks and functions
  const loadProviders = useCallback(async () => {
    try {
      const [providersData, authData, statusData] = await Promise.all([
        listProviders(),
        getAuthStatus(),
        getStatus(),
      ])
      setProviders(providersData)
      setAuthFlow(authData)
      setStatus(statusData)
    } catch {
      showToast("Failed to load providers", "error")
    } finally {
      setLoading(false)
    }
  }, [showToast])

  useEffect(() => {
    loadProviders()
    // Add your existing polling logic here
  }, [loadProviders])

  // Keep all your existing handler functions
  const handleActivateProvider = async (providerId: string) => {
    try {
      await activateProvider(providerId)
      showToast("Provider activated", "success")
      loadProviders()
    } catch {
      showToast("Failed to activate provider", "error")
    }
  }

  const handleDeactivateProvider = async (providerId: string) => {
    try {
      await deactivateProvider(providerId)
      showToast("Provider deactivated", "success")
      loadProviders()
    } catch {
      showToast("Failed to deactivate provider", "error")
    }
  }

  if (loading) {
    return (
      <Box
        display="flex"
        justifyContent="center"
        alignItems="center"
        minHeight="200px"
      >
        <CircularProgress />
      </Box>
    )
  }

  return (
    <Box sx={{ maxWidth: 1200, mx: "auto" }}>
      <Typography variant="h4" component="h1" gutterBottom>
        Providers
      </Typography>

      <Typography variant="body1" color="text.secondary" sx={{ mb: 4 }}>
        Manage your LLM provider configurations and authentication.
      </Typography>

      <MaterialAuthFlowBanner authFlow={authFlow} providers={providers} />

      {providers.map((provider) => (
        <Card key={provider.id} sx={{ mb: 2 }}>
          <CardContent>
            <Box
              sx={{
                display: "flex",
                alignItems: "center",
                justifyContent: "space-between",
              }}
            >
              <Box sx={{ flex: 1 }}>
                <Typography variant="h6" component="h2">
                  {provider.name}
                </Typography>
                <Box
                  sx={{ display: "flex", alignItems: "center", gap: 1, mt: 1 }}
                >
                  <Chip
                    label={provider.id}
                    size="small"
                    color="primary"
                    variant="outlined"
                  />
                  <Chip
                    label={provider.status === "active" ? "Active" : "Inactive"}
                    size="small"
                    color={provider.status === "active" ? "success" : "default"}
                    variant={
                      provider.status === "active" ? "filled" : "outlined"
                    }
                  />
                </Box>
                {provider.description && (
                  <Typography
                    variant="body2"
                    color="text.secondary"
                    sx={{ mt: 1 }}
                  >
                    {provider.description}
                  </Typography>
                )}
              </Box>

              <Box sx={{ display: "flex", alignItems: "center", gap: 1 }}>
                <FormControlLabel
                  control={
                    <Switch
                      checked={provider.status === "active"}
                      onChange={() =>
                        provider.status === "active" ?
                          handleDeactivateProvider(provider.id)
                        : handleActivateProvider(provider.id)
                      }
                      color="primary"
                    />
                  }
                  label=""
                />
                <IconButton size="small" color="primary">
                  <SettingsIcon />
                </IconButton>
              </Box>
            </Box>
          </CardContent>
        </Card>
      ))}

      <Button variant="contained" startIcon={<AddIcon />} sx={{ mt: 3 }}>
        Add Provider
      </Button>
    </Box>
  )
}
