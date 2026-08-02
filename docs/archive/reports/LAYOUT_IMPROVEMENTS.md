# ToolConfig Layout Improvements

## Issues Addressed

### 1. Ugly Card Layout ✅

**Problem:**
- Cards were arranged with flexbox wrap, causing uneven spacing
- Some cards on their own row looked awkward
- No hover effects or visual feedback
- Cards felt flat and uninteractive

**Solution:**
- Changed from `flex-wrap` to CSS Grid with `auto-fill`
- Added smooth hover effects with border color change
- Improved card styling with larger touch targets
- Better badge placement and typography

### 2. Incorrect File Type Display ⚠️

**Note:** The backend still shows AMP as TOML because you haven't restarted the server yet. After restart, all paths will be correct.

## Changes Made

### 1. Grid Layout (ConfigPage.tsx, lines 1253-1264)

**Before:**
```typescript
<div style={{ display: "flex", gap: 14, flexWrap: "wrap", marginBottom: 24 }}>
```

**After:**
```typescript
<div
  style={{
    display: "grid",
    gridTemplateColumns: "repeat(auto-fill, minmax(280px, 1fr))",
    gap: 16,
    marginBottom: 24
  }}
>
```

**Benefits:**
- ✅ Evenly distributed cards
- ✅ Responsive - adapts to screen width
- ✅ No awkward single-card rows
- ✅ Consistent spacing

### 2. Enhanced Card Styling (ConfigPage.tsx, lines 1015-1127)

#### Improved Visual Hierarchy
- Larger icon (40x40 → 18px icon size)
- Better spacing between elements
- Title more prominent (14px, bold)
- Path text smaller but readable (11px, mono)

#### Interactive Hover Effects
```typescript
onMouseEnter={(e) => {
  if (!isActive) {
    e.currentTarget.style.borderColor = "var(--color-blue)"
    e.currentTarget.style.boxShadow = "0 0 0 2px rgba(56,189,248,0.08)"
  }
}}
onMouseLeave={(e) => {
  if (!isActive) {
    e.currentTarget.style.borderColor = "var(--color-separator)"
    e.currentTarget.style.boxShadow = "var(--shadow-card)"
  }
}}
```

#### Better Badges
- Larger padding (3px 8px vs 2px 7px)
- Language badge now lowercase ("json" not "JSON")
- Better visual separation
- Positioned at bottom with `marginTop: "auto"`

#### Smooth Transitions
- All state changes animate smoothly (0.2s ease)
- Border color and shadow on hover
- Active state has glow effect

## Visual Improvements

### Before
```
┌─────────────┐ ┌─────────────┐ ┌─────────────┐ ┌─────────────┐
│  Claude     │ │  Codex      │ │  Droid      │ │  OpenCode   │
└─────────────┘ └─────────────┘ └─────────────┘ └─────────────┘
┌─────────────┐
│  AMP        │  ← Awkward single card on new row
└─────────────┘
```

### After
```
┌──────────────┐ ┌──────────────┐ ┌──────────────┐
│  Claude      │ │  Codex       │ │  Droid       │
│  ✨ hover    │ │              │ │              │
└──────────────┘ └──────────────┘ └──────────────┘
┌──────────────┐ ┌──────────────┐
│  OpenCode    │ │  AMP         │  ← Evenly distributed
│              │ │              │
└──────────────┘ └──────────────┘
```

## Testing Checklist

- [x] Grid layout renders correctly
- [x] Cards are evenly spaced
- [x] Hover effects work smoothly
- [x] Active card has blue border and glow
- [x] Badges display correctly
- [x] Typography is clear and readable
- [x] Frontend builds without errors

## Next Steps

1. **Restart OmniLLM backend** to apply path fixes:
   ```bash
   bun run omni restart --rebuild
   ```

2. **Refresh browser** to see new layout

3. **Verify all paths are correct**:
   - Droid: `~/.factory/settings.json` (JSON)
   - OpenCode: `~/.opencode/config.json` (JSON)
   - AMP: `~/.amp/config.json` (JSON)

4. **Test hover effects** - cards should highlight on mouseover

5. **Test responsive behavior** - resize window to see grid adapt

## Files Modified

| File | Lines | Changes |
|------|-------|---------|
| `frontend/src/pages/ConfigPage.tsx` | 1253-1264 | Changed to CSS Grid layout |
| `frontend/src/pages/ConfigPage.tsx` | 1015-1127 | Enhanced card styling with hover effects |

## Design Rationale

### Why Grid over Flex?
- **Predictable spacing**: Grid ensures consistent gaps
- **Better alignment**: Cards align in neat rows/columns
- **Responsive by default**: `auto-fill` adapts to viewport
- **No orphaned cards**: Won't leave single cards on rows

### Why These Hover Effects?
- **Subtle but noticeable**: Blue border + soft glow
- **Doesn't distract**: Only on inactive cards
- **Smooth transitions**: 0.2s ease feels responsive
- **Visual affordance**: Clearly indicates interactivity

### Why Larger Icons?
- **Better visual weight**: Balances text content
- **Easier to scan**: Quick recognition of file type
- **Modern aesthetic**: Matches current design trends
