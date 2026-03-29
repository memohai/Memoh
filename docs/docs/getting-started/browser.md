# Browser Contexts

Memoh can give a bot access to a headless browser through the **Browser Gateway**. A **Browser Context** stores the browser environment a bot should use, such as viewport size, locale, timezone, and mobile behavior.

Once a browser context is assigned to a bot, the bot can use browser tools to open pages, click elements, fill forms, capture screenshots, and inspect page content.

---

## Concept: Browser Gateway

The Browser Gateway is powered by **Playwright** and provides browser automation for bots. In practice, a browser context acts like a reusable browser profile configuration for one or more bots.

Typical use cases include:

- Navigating websites
- Clicking buttons and links
- Filling and submitting forms
- Reading rendered page content
- Capturing screenshots or PDFs

---

## Creating a Browser Context

Manage contexts from the **Browser Contexts** page in the sidebar.

1. Navigate to the **Browser Contexts** page.
2. Click **Add Browser Context**.
3. Fill in the following field:
   - **Name**: A display name for this browser context.
4. Click **Create**.

---

## Configuring a Browser Context

After creating a context, select it from the sidebar and update its settings.

| Field | Description |
|-------|-------------|
| **Name** | The display name shown in the UI. |
| **Core** | Browser engine: `chromium` (default) or `firefox`. |
| **Viewport Width** | Browser viewport width in pixels. |
| **Viewport Height** | Browser viewport height in pixels. |
| **User Agent** | Optional custom browser user agent string. |
| **Device Scale Factor** | Optional device pixel ratio. |
| **Locale** | Optional locale such as `en-US` or `zh-CN`. |
| **Timezone ID** | Optional timezone such as `UTC` or `Asia/Shanghai`. |
| **Is Mobile** | Enables mobile-style browser behavior. |
| **Ignore HTTPS Errors** | Allows navigation to sites with invalid HTTPS certificates. |

### Managing Contexts

- **Edit**: Select a context and update its configuration.
- **Delete**: Remove a context you no longer use.

---

## Assigning a Browser Context to a Bot

1. Navigate to the **Bots** page and open your bot.
2. Go to the **General** tab.
3. Find the **Browser Context** dropdown.
4. Select the context you created.
5. Click **Save**.

After saving, the bot can use that browser context when browser tools are invoked.

---

## Bot Interaction

When a browser context is configured, the bot can use built-in browser tools such as:

- `browser_action`: perform actions like navigation, click, fill, select, scroll, tab management, screenshot, or PDF export
- `browser_observe`: inspect the current page and gather information for the model

This lets the bot interact with real websites instead of relying only on static HTML or search results.

---

## Browser Core Selection

Memoh's browser image can include Chromium, Firefox, or both. The available cores are determined at build time by the `BROWSER_CORES` build argument.

The install script prompts for browser core selection during setup. To rebuild manually with specific cores:

```bash
BROWSER_CORES=chromium docker compose --profile browser build browser
```

Valid values for `BROWSER_CORES`: `chromium`, `firefox`, `chromium,firefox` (default).

---

## Next Steps

- If you have not configured memory yet, continue with [Built-in Memory Provider](/memory-providers/builtin.md).
- To manage a bot's long-term knowledge after setup, see [Bot Memory Management](/getting-started/memory.md).
