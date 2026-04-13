import {
  Add as AddIcon,
  Link as LinkIcon,
  ExpandMore as ExpandMoreIcon,
  Security as SecurityIcon,
  TrendingUp as TrendingUpIcon,
  ViewModule as ViewModuleIcon,
  Check as CheckIcon,
  Warning as WarningIcon,
  Speed as SpeedIcon,
} from "@mui/icons-material"
// Material UI components
import {
  Box,
  Card,
  CardContent,
  Typography,
  Button,
  Switch,
  Alert,
  Chip,
  CircularProgress,
  Paper,
  Accordion,
  AccordionSummary,
  AccordionDetails,
  TextField,
  Select,
  MenuItem,
  FormControl,
  InputLabel,
  Dialog,
  DialogTitle,
  DialogContent,
  DialogActions,
  List,
  ListItem,
  ListItemText,
  ListItemSecondaryAction,
  ListItemIcon,
  LinearProgress,
  Avatar,
  Stack,
  Skeleton,
  Collapse,
  Divider,
  Grid,
} from "@mui/material"
import { alpha } from "@mui/material/styles"
import { useCallback, useEffect, useRef, useState } from "react"

// Import all your existing API functions - keeping all the same functionality
import {
  activateProvider,
  addProviderInstance,
  authProvider,
  deactivateProvider,
  deleteProvider,
  getAuthStatus,
  getProviderModels,
  getProviderUsage,
  listProviders,
  getStatus,
  toggleProviderModel,
  getProviderPriorities,
  type AuthFlow,
  type Model,
  type Provider,
  type Status,
  type UsageData,
} from "@/api"
import { getDeviceAuthCopy } from "@/lib/device-auth"

interface MaterialProvidersPageCompleteProps {
  showToast: (msg: string, type?: "success" | "error") => void
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
          <CircularProgress size={20} color="inherit" />
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
                gutterBottom
              >
                {authCopy.codeLabel}
              </Typography>
              <Paper
                elevation={0}
                sx={{
                  p: 1.5,
                  display: "inline-block",
                  bgcolor: alpha("#ff9800", 0.1),
                  border: 1,
                  borderColor: alpha("#ff9800", 0.3),
                }}
              >
                <Typography
                  variant="h6"
                  component="code"
                  sx={{
                    fontFamily: "monospace",
                    letterSpacing: "0.2em",
                    fontWeight: 700,
                    color: "#ff9800",
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
                gutterBottom
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
          <Box sx={{ mt: 2, display: "flex", alignItems: "center", gap: 1 }}>
            <CircularProgress size={16} color="inherit" />
            <Typography variant="body2" color="text.secondary">
              {authCopy.waitingLabel}
            </Typography>
          </Box>
        </Box>
      )}
    </Alert>
  )
}

// Material Design provider type icons
const PROVIDER_ICONS = {
  "github-copilot": "🐙",
  antigravity: "🌐",
  alibaba: "☁️",
  "azure-openai": "🔵",
}

const PROVIDER_COLORS = {
  "github-copilot": "#0a84ff",
  antigravity: "#30d158",
  alibaba: "#ff9f0a",
  "azure-openai": "#0078d4",
  google: "#4285f4",
}

const PROVIDER_TYPES = [
  {
    id: "github-copilot",
    name: "GitHub Copilot",
    desc: "Access Copilot models via OAuth or token",
  },
  {
    id: "antigravity",
    name: "Antigravity (Google)",
    desc: "Google Vertex AI via OAuth client credentials",
  },
  {
    id: "alibaba",
    name: "Alibaba DashScope",
    desc: "Qwen models via API key or OAuth",
  },
  {
    id: "azure-openai",
    name: "Azure OpenAI",
    desc: "Azure OpenAI Service with your own deployments",
  },
  {
    id: "google",
    name: "Google Gemini",
    desc: "Google Gemini API with your API key",
  },
]

// Material Design Auth Forms
function MaterialAuthForm({
  provider,
  onSubmit,
  onCancel,
}: {
  provider: Provider
  onSubmit: (body: Record<string, string>) => Promise<void>
  onCancel: () => void
}) {
  const [values, setValues] = useState<Record<string, string>>({})
  const [loading, setLoading] = useState(false)

  const handleSubmit = async () => {
    setLoading(true)
    try {
      await onSubmit(values)
      setValues({})
    } finally {
      setLoading(false)
    }
  }

  const renderForm = () => {
    switch (provider.type) {
      case "github-copilot": {
        return (
          <>
            <FormControl fullWidth sx={{ mb: 2 }}>
              <InputLabel>Auth Method</InputLabel>
              <Select
                value={values.method || "oauth"}
                onChange={(e) =>
                  setValues({ ...values, method: e.target.value })
                }
                label="Auth Method"
              >
                <MenuItem value="oauth">OAuth device flow (browser)</MenuItem>
                <MenuItem value="token">Paste existing token</MenuItem>
              </Select>
            </FormControl>
            {values.method === "token" && (
              <TextField
                fullWidth
                label="GitHub Token"
                type="password"
                placeholder="ghu_…"
                value={values.token || ""}
                onChange={(e) =>
                  setValues({ ...values, token: e.target.value })
                }
                sx={{ mb: 2 }}
              />
            )}
          </>
        )
      }

      case "alibaba": {
        return (
          <>
            <FormControl fullWidth sx={{ mb: 2 }}>
              <InputLabel>Auth Method</InputLabel>
              <Select
                value={values.method || "api-key"}
                onChange={(e) =>
                  setValues({ ...values, method: e.target.value })
                }
                label="Auth Method"
              >
                <MenuItem value="api-key">API Key</MenuItem>
                <MenuItem value="oauth">OAuth (device flow)</MenuItem>
              </Select>
            </FormControl>
            {values.method === "api-key" && (
              <>
                <FormControl fullWidth sx={{ mb: 2 }}>
                  <InputLabel>API Mode</InputLabel>
                  <Select
                    value={values.plan || "standard"}
                    onChange={(e) =>
                      setValues({ ...values, plan: e.target.value })
                    }
                    label="API Mode"
                  >
                    <MenuItem value="standard">
                      Standard (pay-as-you-go, recommended for qwen3.6-plus)
                    </MenuItem>
                    <MenuItem value="coding-plan">Coding Plan</MenuItem>
                  </Select>
                </FormControl>
                <FormControl fullWidth sx={{ mb: 2 }}>
                  <InputLabel>Region</InputLabel>
                  <Select
                    value={values.region || "global"}
                    onChange={(e) =>
                      setValues({ ...values, region: e.target.value })
                    }
                    label="Region"
                  >
                    <MenuItem value="global">
                      Global (dashscope-intl.aliyuncs.com)
                    </MenuItem>
                    <MenuItem value="china">
                      China (dashscope.aliyuncs.com)
                    </MenuItem>
                  </Select>
                </FormControl>
                <TextField
                  fullWidth
                  label="Base URL (optional)"
                  placeholder={
                    values.plan === "coding-plan" ?
                      "https://coding-intl.dashscope.aliyuncs.com/v1"
                    : "https://dashscope-intl.aliyuncs.com/compatible-mode/v1"
                  }
                  value={values.endpoint || ""}
                  onChange={(e) =>
                    setValues({ ...values, endpoint: e.target.value })
                  }
                  sx={{ mb: 2 }}
                />
                <TextField
                  fullWidth
                  label="DashScope API Key"
                  type="password"
                  placeholder={
                    values.plan === "coding-plan" ? "sk-sp-…" : "sk-…"
                  }
                  value={values.apiKey || ""}
                  onChange={(e) =>
                    setValues({ ...values, apiKey: e.target.value })
                  }
                  sx={{ mb: 2 }}
                />
              </>
            )}
          </>
        )
      }

      case "azure-openai": {
        return (
          <TextField
            fullWidth
            label="API Key"
            type="password"
            placeholder="Enter your Azure OpenAI API key"
            value={values.apiKey || ""}
            onChange={(e) => setValues({ ...values, apiKey: e.target.value })}
            sx={{ mb: 2 }}
            helperText="Enter your Azure OpenAI API key. Configure endpoint and deployments after authentication."
          />
        )
      }

      case "antigravity": {
        return (
          <>
            <TextField
              fullWidth
              label="OAuth Client ID"
              placeholder="…apps.googleusercontent.com"
              value={values.clientId || ""}
              onChange={(e) =>
                setValues({ ...values, clientId: e.target.value })
              }
              sx={{ mb: 2 }}
            />
            <TextField
              fullWidth
              label="OAuth Client Secret"
              type="password"
              placeholder="GOCSPX-…"
              value={values.clientSecret || ""}
              onChange={(e) =>
                setValues({ ...values, clientSecret: e.target.value })
              }
              sx={{ mb: 2 }}
              helperText="Opens a Google OAuth browser flow once submitted."
            />
          </>
        )
      }

      default: {
        return null
      }
    }
  }

  return (
    <Paper elevation={0} sx={{ p: 3, bgcolor: alpha("#2196f3", 0.05) }}>
      <Typography variant="h6" gutterBottom sx={{ color: "#2196f3" }}>
        Authenticate {provider.name}
      </Typography>
      {renderForm()}
      <Stack direction="row" spacing={1} sx={{ mt: 2 }}>
        <Button
          variant="contained"
          onClick={handleSubmit}
          disabled={loading}
          startIcon={
            loading ? <CircularProgress size={16} /> : <SecurityIcon />
          }
        >
          {loading ? "Submitting..." : "Submit"}
        </Button>
        <Button variant="text" onClick={onCancel} disabled={loading}>
          Cancel
        </Button>
      </Stack>
    </Paper>
  )
}

// Material Design Models Dialog
function MaterialModelsDialog({
  provider,
  onModelsChanged,
}: {
  provider: Provider
  onModelsChanged?: () => void
}) {
  const [open, setOpen] = useState(false)
  const [models, setModels] = useState<Array<Model> | null>(null)
  const [loading, setLoading] = useState(false)
  const [search, setSearch] = useState("")

  const loadModels = async () => {
    setLoading(true)
    try {
      const response = await getProviderModels(provider.id)
      setModels(response.models)
    } catch (error) {
      console.error("Failed to load models:", error)
    } finally {
      setLoading(false)
    }
  }

  const handleOpen = () => {
    setOpen(true)
    if (!models) loadModels()
  }

  const handleToggle = async (model: Model) => {
    if (!models) return
    const newEnabled = !model.enabled
    // Optimistic update
    setModels(
      (prev) =>
        prev?.map((m) =>
          m.id === model.id ? { ...m, enabled: newEnabled } : m,
        ) || null,
    )
    try {
      await toggleProviderModel(provider.id, model.id, newEnabled)
      onModelsChanged?.()
    } catch {
      // Revert on error
      setModels(
        (prev) =>
          prev?.map((m) =>
            m.id === model.id ? { ...m, enabled: model.enabled } : m,
          ) || null,
      )
    }
  }

  const filteredModels =
    models?.filter((m) => m.id.toLowerCase().includes(search.toLowerCase()))
    || []

  const enabledCount = models?.filter((m) => m.enabled).length || 0

  return (
    <>
      <Button
        variant="text"
        size="small"
        startIcon={<ViewModuleIcon />}
        onClick={handleOpen}
        endIcon={
          provider.totalModelCount ?
            <Chip
              label={`${provider.enabledModelCount}/${provider.totalModelCount}`}
              size="small"
              variant="outlined"
            />
          : null
        }
      >
        Models
      </Button>

      <Dialog
        open={open}
        onClose={() => setOpen(false)}
        maxWidth="md"
        fullWidth
      >
        <DialogTitle>
          <Stack
            direction="row"
            justifyContent="space-between"
            alignItems="center"
          >
            <Box>
              <Typography variant="h6">{provider.name} — Models</Typography>
              {models && (
                <Typography variant="caption" color="text.secondary">
                  <Box component="span" sx={{ color: "success.main" }}>
                    {enabledCount}
                  </Box>{" "}
                  of {models.length} enabled
                </Typography>
              )}
            </Box>
          </Stack>
        </DialogTitle>
        <DialogContent>
          {loading && (
            <Box sx={{ display: "flex", justifyContent: "center", py: 4 }}>
              <CircularProgress />
            </Box>
          )}
          {models && !loading && (
            <Stack spacing={2}>
              <TextField
                fullWidth
                placeholder="Filter models…"
                value={search}
                onChange={(e) => setSearch(e.target.value)}
                size="small"
              />
              <List>
                {filteredModels.map((model) => (
                  <ListItem key={model.id} divider>
                    <ListItemIcon>
                      <Avatar
                        sx={{
                          width: 32,
                          height: 32,
                          bgcolor: model.enabled ? "success.light" : "grey.300",
                          fontSize: 12,
                        }}
                      >
                        {model.enabled ?
                          <CheckIcon fontSize="small" />
                        : "M"}
                      </Avatar>
                    </ListItemIcon>
                    <ListItemText
                      primary={
                        <Typography variant="body2" fontFamily="monospace">
                          {model.id}
                        </Typography>
                      }
                      secondary={
                        model.name !== model.id ? model.name : undefined
                      }
                    />
                    <ListItemSecondaryAction>
                      <Switch
                        checked={model.enabled}
                        onChange={() => handleToggle(model)}
                        color="primary"
                      />
                    </ListItemSecondaryAction>
                  </ListItem>
                ))}
              </List>
            </Stack>
          )}
        </DialogContent>
        <DialogActions>
          <Button onClick={() => setOpen(false)}>Done</Button>
        </DialogActions>
      </Dialog>
    </>
  )
}

// Material Design Usage Dialog
function MaterialUsageDialog({ provider }: { provider: Provider }) {
  const [open, setOpen] = useState(false)
  const [data, setData] = useState<UsageData | null>(null)
  const [loading, setLoading] = useState(false)

  const loadUsage = async () => {
    setLoading(true)
    try {
      const result = await getProviderUsage(provider.id)
      setData(result)
    } catch (error) {
      console.error("Failed to load usage:", error)
    } finally {
      setLoading(false)
    }
  }

  const handleOpen = () => {
    setOpen(true)
    if (!data) loadUsage()
  }

  return (
    <>
      <Button
        variant="text"
        size="small"
        startIcon={<TrendingUpIcon />}
        onClick={handleOpen}
      >
        Usage
      </Button>

      <Dialog
        open={open}
        onClose={() => setOpen(false)}
        maxWidth="sm"
        fullWidth
      >
        <DialogTitle>{provider.name} — Usage</DialogTitle>
        <DialogContent>
          {loading && (
            <Box sx={{ display: "flex", justifyContent: "center", py: 4 }}>
              <CircularProgress />
            </Box>
          )}
          {data && !loading && (
            <Stack spacing={3}>
              {/* Quota snapshots */}
              {data.quota_snapshots
                && Object.entries(data.quota_snapshots).map(([key, quota]) => (
                  <Paper
                    key={key}
                    elevation={0}
                    sx={{
                      p: 2,
                      bgcolor: "background.default",
                      border: 1,
                      borderColor: "divider",
                    }}
                  >
                    <Typography variant="subtitle2" gutterBottom>
                      {key.replaceAll("_", " ").toUpperCase()}
                    </Typography>
                    <Box sx={{ mb: 1 }}>
                      <LinearProgress
                        variant="determinate"
                        value={
                          quota.unlimited ? 100 : 100 - quota.percent_remaining
                        }
                        sx={{ height: 8, borderRadius: 1 }}
                        color={
                          quota.unlimited ? "info"
                          : quota.percent_remaining < 25 ?
                            "error"
                          : quota.percent_remaining < 50 ?
                            "warning"
                          : "success"
                        }
                      />
                    </Box>
                    <Typography variant="caption" color="text.secondary">
                      {quota.unlimited ?
                        "Unlimited usage"
                      : `${Math.round(100 - quota.percent_remaining)}% used · ${quota.remaining.toLocaleString()} remaining`
                      }
                    </Typography>
                  </Paper>
                ))}
              {/* Raw data fallback */}
              {(!data.quota_snapshots
                || Object.keys(data.quota_snapshots).length === 0) && (
                <Paper
                  elevation={0}
                  sx={{
                    p: 2,
                    bgcolor: "background.default",
                    border: 1,
                    borderColor: "divider",
                  }}
                >
                  <Typography
                    variant="caption"
                    color="text.secondary"
                    gutterBottom
                  >
                    Raw Data
                  </Typography>
                  <Typography
                    variant="body2"
                    component="pre"
                    sx={{
                      fontFamily: "monospace",
                      fontSize: "0.75rem",
                      overflow: "auto",
                      maxHeight: 200,
                    }}
                  >
                    {JSON.stringify(data, null, 2)}
                  </Typography>
                </Paper>
              )}
            </Stack>
          )}
        </DialogContent>
        <DialogActions>
          <Button onClick={loadUsage} disabled={loading}>
            Refresh
          </Button>
          <Button onClick={() => setOpen(false)}>Done</Button>
        </DialogActions>
      </Dialog>
    </>
  )
}

// Material Design Provider Card
function MaterialProviderCard({
  provider,
  isFlowRunning,
  isActivating,
  onActivate,
  onDeactivate,
  onDelete,
  onAuthSubmit,
  onModelsChanged,
  priorityIndex,
  multiProvider,
}: {
  provider: Provider
  isFlowRunning: boolean
  isActivating: boolean
  onActivate: (id: string) => void
  onDeactivate: (id: string) => void
  onDelete: (id: string) => void
  onAuthSubmit: (id: string, body: Record<string, string>) => Promise<void>
  onModelsChanged: () => void
  priorityIndex: number
  multiProvider: boolean
}) {
  const [showAuthForm, setShowAuthForm] = useState(false)
  const providerColor = PROVIDER_COLORS[provider.type] || "#0a84ff"

  const handleAuthSubmit = async (body: Record<string, string>) => {
    await onAuthSubmit(provider.id, body)
    setShowAuthForm(false)
  }

  return (
    <Card
      elevation={provider.isActive ? 3 : 1}
      sx={{
        position: "relative",
        border: provider.isActive ? 2 : 1,
        borderColor: provider.isActive ? "success.light" : "divider",
        "&::before":
          provider.isActive ?
            {
              content: '""',
              position: "absolute",
              top: 0,
              left: 0,
              right: 0,
              height: 4,
              bgcolor: providerColor,
              borderRadius: "4px 4px 0 0",
            }
          : {},
      }}
    >
      <CardContent sx={{ pt: provider.isActive ? 3 : 2 }}>
        <Grid container spacing={2} alignItems="flex-start">
          {/* Left: Icon and info */}
          <Grid sx={{ flex: 1 }}>
            <Stack direction="row" spacing={2} alignItems="flex-start">
              <Avatar
                sx={{
                  bgcolor: alpha(providerColor, 0.1),
                  border: `1px solid ${alpha(providerColor, 0.3)}`,
                  color: providerColor,
                  width: 48,
                  height: 48,
                }}
              >
                {PROVIDER_ICONS[provider.type] || "◌"}
              </Avatar>
              <Box sx={{ minWidth: 0, flex: 1 }}>
                <Typography variant="h6" component="h2" noWrap>
                  {provider.name}
                </Typography>
                <Stack direction="row" spacing={1} sx={{ mt: 0.5, mb: 1 }}>
                  <Chip
                    label={provider.id}
                    size="small"
                    variant="outlined"
                    sx={{ fontSize: "0.6rem", fontFamily: "monospace" }}
                  />
                  {provider.isActive && (
                    <Chip
                      label="Active"
                      size="small"
                      color="success"
                      variant="filled"
                      icon={<CheckIcon />}
                    />
                  )}
                  {!provider.isActive
                    && provider.authStatus === "authenticated" && (
                      <Chip
                        label="Ready"
                        size="small"
                        color="primary"
                        variant="outlined"
                      />
                    )}
                  {provider.authStatus === "unauthenticated" && (
                    <Chip
                      label="Not authorized"
                      size="small"
                      color="warning"
                      variant="outlined"
                    />
                  )}
                  {provider.isActive && multiProvider && priorityIndex >= 0 && (
                    <Chip
                      label={`#${priorityIndex + 1}`}
                      size="small"
                      color="primary"
                      variant="filled"
                      icon={<SpeedIcon />}
                    />
                  )}
                </Stack>
                {/* Model progress */}
                {provider.authStatus === "authenticated"
                  && provider.totalModelCount != null
                  && provider.totalModelCount > 0 && (
                    <Box sx={{ mt: 1 }}>
                      <Typography
                        variant="caption"
                        color="text.secondary"
                        gutterBottom
                      >
                        Models enabled:{" "}
                        <Box
                          component="span"
                          sx={{ color: "success.main", fontWeight: 600 }}
                        >
                          {provider.enabledModelCount}
                        </Box>{" "}
                        / {provider.totalModelCount}
                      </Typography>
                      <LinearProgress
                        variant="determinate"
                        value={
                          ((provider.enabledModelCount || 0)
                            / provider.totalModelCount)
                          * 100
                        }
                        sx={{ height: 4, borderRadius: 1 }}
                        color="success"
                      />
                    </Box>
                  )}
              </Box>
            </Stack>
          </Grid>

          {/* Right: Actions */}
          <Grid>
            <Stack spacing={1}>
              <Stack direction="row" spacing={1}>
                {provider.isActive ?
                  <Button
                    variant="outlined"
                    size="small"
                    disabled={isFlowRunning || isActivating}
                    onClick={() => onDeactivate(provider.id)}
                    startIcon={
                      isActivating ? <CircularProgress size={16} /> : undefined
                    }
                  >
                    {isActivating ? "Working…" : "Deactivate"}
                  </Button>
                : <Button
                    variant="contained"
                    size="small"
                    color="success"
                    disabled={
                      isFlowRunning
                      || isActivating
                      || provider.authStatus !== "authenticated"
                    }
                    onClick={() => onActivate(provider.id)}
                    startIcon={
                      isActivating ?
                        <CircularProgress size={16} />
                      : <CheckIcon />
                    }
                  >
                    {isActivating ? "Working…" : "Activate"}
                  </Button>
                }
                <Button
                  variant="text"
                  size="small"
                  disabled={isFlowRunning}
                  onClick={() => setShowAuthForm(!showAuthForm)}
                  startIcon={<SecurityIcon />}
                >
                  {showAuthForm ? "Cancel" : "Authorize"}
                </Button>
              </Stack>
              <Stack direction="row" spacing={1}>
                <MaterialModelsDialog
                  provider={provider}
                  onModelsChanged={onModelsChanged}
                />
                <MaterialUsageDialog provider={provider} />
              </Stack>
            </Stack>
          </Grid>
        </Grid>
      </CardContent>

      {/* Auth form */}
      <Collapse in={showAuthForm}>
        <Divider />
        <Box sx={{ p: 2 }}>
          <MaterialAuthForm
            provider={provider}
            onSubmit={handleAuthSubmit}
            onCancel={() => setShowAuthForm(false)}
          />
        </Box>
      </Collapse>
    </Card>
  )
}

// Material Design Add Provider Dialog
function MaterialAddProviderDialog({
  onAdd,
  disabled,
}: {
  onAdd: (type: string) => void
  disabled: boolean
}) {
  const [open, setOpen] = useState(false)

  return (
    <>
      <Button
        variant="contained"
        startIcon={<AddIcon />}
        disabled={disabled}
        onClick={() => setOpen(true)}
        sx={{ borderRadius: 2 }}
      >
        Add Provider
      </Button>

      <Dialog
        open={open}
        onClose={() => setOpen(false)}
        maxWidth="sm"
        fullWidth
      >
        <DialogTitle>Add Provider</DialogTitle>
        <DialogContent>
          <Typography variant="body2" color="text.secondary" gutterBottom>
            Select a provider type. You can add multiple accounts of the same
            type.
          </Typography>
          <Stack spacing={1} sx={{ mt: 2 }}>
            {PROVIDER_TYPES.map((pt) => {
              const providerColor = PROVIDER_COLORS[pt.id] || "#0a84ff"
              return (
                <Card
                  key={pt.id}
                  variant="outlined"
                  sx={{
                    cursor: "pointer",
                    transition: "all 0.2s",
                    "&:hover": {
                      bgcolor: alpha(providerColor, 0.05),
                      borderColor: alpha(providerColor, 0.3),
                    },
                  }}
                  onClick={() => {
                    onAdd(pt.id)
                    setOpen(false)
                  }}
                >
                  <CardContent>
                    <Stack direction="row" spacing={2} alignItems="center">
                      <Avatar
                        sx={{
                          bgcolor: alpha(providerColor, 0.1),
                          border: `1px solid ${alpha(providerColor, 0.3)}`,
                          color: providerColor,
                        }}
                      >
                        {PROVIDER_ICONS[pt.id] || "◌"}
                      </Avatar>
                      <Box sx={{ flex: 1 }}>
                        <Typography variant="subtitle1" fontWeight={600}>
                          {pt.name}
                        </Typography>
                        <Typography variant="body2" color="text.secondary">
                          {pt.desc}
                        </Typography>
                      </Box>
                    </Stack>
                  </CardContent>
                </Card>
              )
            })}
          </Stack>
        </DialogContent>
        <DialogActions>
          <Button onClick={() => setOpen(false)}>Cancel</Button>
        </DialogActions>
      </Dialog>
    </>
  )
}

// Main Material Providers Page
export function MaterialProvidersPageComplete({
  showToast,
}: MaterialProvidersPageCompleteProps) {
  const [providers, setProviders] = useState<Array<Provider>>([])
  const [status, setStatus] = useState<Status | null>(null)
  const [loading, setLoading] = useState(true)
  const [activating, setActivating] = useState<string | null>(null)
  const [priorities, setPriorities] = useState<Record<string, number>>({})
  const [expandedGroups, setExpandedGroups] = useState<Record<string, boolean>>(
    {},
  )
  const pollTimer = useRef<ReturnType<typeof setInterval> | null>(null)

  const load = useCallback(async () => {
    try {
      const [p, s, pri] = await Promise.all([
        listProviders(),
        getStatus(),
        getProviderPriorities(),
      ])
      setProviders(p)
      setStatus(s)
      setPriorities(pri.priorities)
    } catch (e) {
      showToast(
        "Failed to load: " + (e instanceof Error ? e.message : String(e)),
        "error",
      )
    } finally {
      setLoading(false)
    }
  }, [showToast])

  const stopPoll = useCallback(() => {
    if (pollTimer.current) {
      clearInterval(pollTimer.current)
      pollTimer.current = null
    }
  }, [])

  const startPoll = useCallback(() => {
    stopPoll()
    pollTimer.current = setInterval(async () => {
      try {
        const af = await getAuthStatus()
        setStatus((prev) => (prev ? { ...prev, authFlow: af } : prev))
        if (af?.status === "complete") {
          stopPoll()
          showToast("Authentication complete!")
          await load()
        } else if (af?.status === "error") {
          stopPoll()
          showToast("Auth failed: " + (af.error ?? "unknown"), "error")
          await load()
        }
      } catch {
        /* ignore */
      }
    }, 2000)
  }, [load, showToast, stopPoll])

  useEffect(() => {
    void load()
    return stopPoll
  }, [load, stopPoll])

  useEffect(() => {
    const authFlow = status?.authFlow
    if (
      authFlow
      && (authFlow.status === "pending" || authFlow.status === "awaiting_user")
    ) {
      startPoll()
    }
  }, [startPoll, status])

  const handleActivate = async (id: string) => {
    setActivating(id)
    try {
      const result = await activateProvider(id)
      if (result.success) {
        showToast(`Activated ${result.provider?.name ?? id}`)
        await load()
      }
    } catch (e) {
      showToast(
        "Activate failed: " + (e instanceof Error ? e.message : String(e)),
        "error",
      )
    } finally {
      setActivating(null)
    }
  }

  const handleDeactivate = async (id: string) => {
    setActivating(id)
    try {
      await deactivateProvider(id)
      showToast("Deactivated")
      await load()
    } catch (e) {
      showToast(
        "Deactivate failed: " + (e instanceof Error ? e.message : String(e)),
        "error",
      )
    } finally {
      setActivating(null)
    }
  }

  const handleDelete = async (id: string) => {
    setActivating(id)
    try {
      const result = await deleteProvider(id)
      showToast(result.message || "Deleted")
      await load()
    } catch (e) {
      showToast(
        "Delete failed: " + (e instanceof Error ? e.message : String(e)),
        "error",
      )
    } finally {
      setActivating(null)
    }
  }

  const handleAuthSubmit = async (id: string, body: Record<string, string>) => {
    try {
      const result = await authProvider(id, body)
      if (result.success) {
        showToast("Authentication successful")
        await load()
      } else if (result.requiresAuth) {
        showToast("Follow the auth instructions above")
        startPoll()
        await load()
      }
    } catch (e) {
      showToast(
        "Auth failed: " + (e instanceof Error ? e.message : String(e)),
        "error",
      )
    }
  }

  const handleAddInstance = async (providerType: string) => {
    try {
      const result = await addProviderInstance(providerType)
      if (result.success && result.provider) {
        showToast(`Created ${result.provider.name}`)
        await load()
      }
    } catch (e) {
      showToast(
        "Add failed: " + (e instanceof Error ? e.message : String(e)),
        "error",
      )
    }
  }

  const isFlowRunning = ["awaiting_user", "pending"].includes(
    status?.authFlow?.status ?? "",
  )

  const activeProviders = providers
    .filter((p) => p.isActive)
    .sort((a, b) => (priorities[a.id] ?? 0) - (priorities[b.id] ?? 0))

  // Group providers by type
  const providerGroups = providers.reduce<Record<string, Array<Provider>>>(
    (g, p) => {
      if (!g[p.type]) g[p.type] = []
      g[p.type].push(p)
      return g
    },
    {},
  )

  if (loading && providers.length === 0) {
    return (
      <Box sx={{ maxWidth: 1200, mx: "auto" }}>
        <Box sx={{ mb: 4 }}>
          <Skeleton variant="text" width={200} height={40} />
          <Skeleton variant="text" width={300} height={20} />
        </Box>
        {Array.from({ length: 3 }).map((_, i) => (
          <Skeleton
            key={i}
            variant="rectangular"
            height={120}
            sx={{ mb: 2, borderRadius: 2 }}
          />
        ))}
      </Box>
    )
  }

  return (
    <Box sx={{ maxWidth: 1200, mx: "auto" }}>
      {/* Header */}
      <Box sx={{ mb: 4 }}>
        <Box
          sx={{
            display: "flex",
            justifyContent: "space-between",
            alignItems: "flex-start",
            mb: 2,
          }}
        >
          <Box>
            <Typography variant="h4" component="h1" gutterBottom>
              Providers
            </Typography>
            <Typography variant="body1" color="text.secondary">
              {activeProviders.length > 0 ?
                <>
                  <Box
                    component="span"
                    sx={{ color: "success.main", fontWeight: 600 }}
                  >
                    {activeProviders.length} active
                  </Box>{" "}
                  · {providers.length} total instances
                </>
              : `${providers.length} instance${providers.length !== 1 ? "s" : ""} · none active`
              }
            </Typography>
          </Box>
          <MaterialAddProviderDialog
            onAdd={handleAddInstance}
            disabled={isFlowRunning}
          />
        </Box>

        <MaterialAuthFlowBanner
          authFlow={status?.authFlow}
          providers={providers}
        />
      </Box>

      {/* Provider Groups */}
      <Stack spacing={3}>
        {PROVIDER_TYPES.map((providerType) => {
          const typeProviders = providerGroups[providerType.id] || []
          const isExpanded = expandedGroups[providerType.id] ?? true
          const providerColor = PROVIDER_COLORS[providerType.id] || "#0a84ff"

          return (
            <Accordion
              key={providerType.id}
              expanded={isExpanded}
              onChange={() =>
                setExpandedGroups((prev) => ({
                  ...prev,
                  [providerType.id]: !prev[providerType.id],
                }))
              }
              sx={{
                "&:before": { display: "none" },
                boxShadow: 1,
                borderRadius: 2,
                "&.Mui-expanded": { margin: 0 },
              }}
            >
              <AccordionSummary expandIcon={<ExpandMoreIcon />}>
                <Stack direction="row" spacing={2} alignItems="center">
                  <Avatar
                    sx={{
                      bgcolor: alpha(providerColor, 0.1),
                      border: `1px solid ${alpha(providerColor, 0.3)}`,
                      color: providerColor,
                      width: 32,
                      height: 32,
                    }}
                  >
                    {PROVIDER_ICONS[providerType.id] || "◌"}
                  </Avatar>
                  <Box>
                    <Typography variant="h6" fontWeight={600}>
                      {providerType.name}
                    </Typography>
                    {typeProviders.length > 0 && (
                      <Typography variant="caption" color="text.secondary">
                        {typeProviders.length}{" "}
                        {typeProviders.length === 1 ? "account" : "accounts"}
                      </Typography>
                    )}
                  </Box>
                </Stack>
              </AccordionSummary>
              <AccordionDetails>
                {typeProviders.length > 0 ?
                  <Stack spacing={2}>
                    {typeProviders.map((provider) => (
                      <MaterialProviderCard
                        key={provider.id}
                        provider={provider}
                        isFlowRunning={isFlowRunning}
                        isActivating={activating === provider.id}
                        onActivate={handleActivate}
                        onDeactivate={handleDeactivate}
                        onDelete={handleDelete}
                        onAuthSubmit={handleAuthSubmit}
                        onModelsChanged={load}
                        priorityIndex={activeProviders.findIndex(
                          (x) => x.id === provider.id,
                        )}
                        multiProvider={activeProviders.length >= 2}
                      />
                    ))}
                  </Stack>
                : <Paper
                    elevation={0}
                    sx={{
                      p: 4,
                      textAlign: "center",
                      bgcolor: alpha(providerColor, 0.05),
                      border: `2px dashed ${alpha(providerColor, 0.2)}`,
                      borderRadius: 2,
                    }}
                  >
                    <Avatar
                      sx={{
                        bgcolor: alpha(providerColor, 0.1),
                        border: `1px solid ${alpha(providerColor, 0.3)}`,
                        color: providerColor,
                        width: 48,
                        height: 48,
                        mx: "auto",
                        mb: 2,
                        opacity: 0.7,
                      }}
                    >
                      {PROVIDER_ICONS[providerType.id] || "◌"}
                    </Avatar>
                    <Typography
                      variant="body2"
                      color="text.secondary"
                      gutterBottom
                    >
                      No {providerType.name} accounts configured
                    </Typography>
                    <Button
                      variant="outlined"
                      size="small"
                      onClick={() => handleAddInstance(providerType.id)}
                      disabled={isFlowRunning}
                      sx={{ mt: 1 }}
                    >
                      Add Account
                    </Button>
                  </Paper>
                }
              </AccordionDetails>
            </Accordion>
          )
        })}
      </Stack>
    </Box>
  )
}
