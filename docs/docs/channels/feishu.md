# Feishu (Lark) Channel Configuration

Memoh supports integrating with Feishu (Lark) via the Open Platform. This allows your enterprise bots to interact with users in groups or private chats.

## Step 1: Create a Feishu App

1. Go to the [Feishu Open Platform](https://open.feishu.cn/app) and log in.
2. Click **Create Custom App**.
3. Enter a name and description, then click **Create**.
4. In the left sidebar, go to **App Settings** > **Credentials & Basic Info**.
5. Copy your **App ID** and **App Secret**.

## Step 2: Enable Bot Capability

1. In the Feishu Developer Console, go to **Capabilities** > **Bot**.
2. Click **Enable Bot**.

## Step 3: Configure Permissions

1. Go to **Capabilities** > **Permissions**.
2. Search for and enable the following permissions (at minimum):
   - `im:message` (Receive and send messages)
   - `im:chat` (Access group chat information)
3. Click **Apply for Permissions** if required by your organization.

## Step 4: Configure Events (Webhook Mode)

1. In Memoh, go to your Bot's **Channels** tab and add a **Feishu** channel.
2. Choose **Inbound Mode**: `webhook`.
3. Fill in your **App ID** and **App Secret**.
4. Click **Save**. Memoh will generate a **Webhook Callback URL**.
5. Copy this URL.
6. In the Feishu Console, go to **App Settings** > **Event Subscriptions**.
7. Paste the URL into the **Verification URL** field and save.
8. Add events like `Receive Message` (im.message.receive_v1).

> Official Guide: [Feishu Custom Bot Guide](https://open.feishu.cn/document/client-docs/bot-v3/add-custom-bot)

## Step 5: Publish Your App

1. In the Feishu Console, go to **App Release** > **Version Management & Release**.
2. Click **Create a Version**, fill in the details, and submit for approval.
3. Once approved and published, the bot is ready to use.
