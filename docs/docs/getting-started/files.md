# Bot Files Management

Every Memoh Bot has its own dedicated filesystem inside its isolated container. You can manage this filesystem directly from the **Files** tab in the Bot Detail page.

---

## Operations

The **FileManager** component provides a familiar file-browsing experience with a toolbar, directory tree, and integrated editor.

### Browsing and Navigation

- **Breadcrumb Navigation**: Quickly move between parent directories.
- **Refresh**: Reload the file list to see the latest changes (e.g., files created by the bot).
- **New Folder**: Create a directory within the current path.

### Managing Files

- **Upload**: Select files from your local computer to transfer them into the bot's container.
- **Rename**: Click on a file or folder and use the rename action to update its identifier.
- **Delete**: Remove files or folders (with recursive support for directories).
- **Download**: Retrieve a file from the bot's container back to your local machine.

---

## Viewing and Editing

The integrated **FileViewer** allows you to interact with the bot's files without leaving the web UI.

### Text Files

- **Read/Edit**: Click a text file (e.g., `.md`, `.js`, `.py`, `.toml`) to open it in the built-in **Monaco Editor**.
- **Syntax Highlighting**: Supports common programming languages and configuration formats.
- **Save**: Modify the file's content and click **Save** to apply the changes to the bot's filesystem.

### Images

- **Preview**: Click an image file (e.g., `.png`, `.jpg`, `.webp`) to see a visual preview directly in the file manager.

---

## Bot Interaction with Files

Remember that the bot itself can also perform these operations:
- Use its **Skills** or **MCP tools** to read, write, and manage its own files.
- The **Files** tab is your portal to monitor and manually intervene in the bot's workspace.
