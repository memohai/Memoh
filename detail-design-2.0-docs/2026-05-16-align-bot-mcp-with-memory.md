# Delivery: Align Bot MCP Rail with Memory (2026-05-16)

## Context
Standardize the visual structure of the MCP Server List rail to align with the Bot Memory file list rail, ensuring a consistent layout across different bot configuration panels.

## Key Changes
- **Layout Alignment**: Updated the L3 rail width to `lg:flex-[0.5] lg:min-w-[230px] lg:max-w-[260px]` to match the Memory component.
- **Simplification**: Removed the mobile collapse functionality (`Menu` button and `isMobileCollapsed` state) as it's no longer required by the new design.
- **Style Unification**: Standardized container padding (`p-3`) and removed redundant max-width/font-family constraints from the root container.
- **Semantic Cleanup**: Cleaned up unused imports (`Menu` from `lucide-vue-next`) and simplified template logic by removing conditional classes dependent on collapse state.

## File Modifications
- `apps/web/src/pages/bots/components/bot-mcp.vue`

## Verification Results
- **Boundary Audit**: Modifications are restricted to the UI component layer.
- **Lint Status**: Verified clean with `eslint --fix`.
