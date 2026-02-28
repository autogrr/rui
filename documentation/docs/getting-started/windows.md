---
sidebar_position: 3
title: Windows
description: Install and run rui on Windows as a background service.
---

# Windows Installation

In this guide we will download rui, set it up, and create a Windows Task so it runs in the background without needing a command prompt window open 24/7.

## Download

1. Download the latest Windows release from [GitHub Releases](https://github.com/autogrr/rui/releases/latest).
   - For most systems, download `qui_x.x.x_windows_amd64.zip`.
2. Extract the archive and place `rui.exe` in a directory, for example `C:\rui`.

:::tip
Avoid placing rui in `C:\Program Files` — it can cause permission issues with the database and config files.
:::

## Initial Setup

1. Open **Command Prompt** or **PowerShell** and navigate to the directory:
   ```powershell
   cd C:\rui
   ```

2. Start rui for the first time to generate the default config and create your account:
   ```powershell
   .\rui.exe serve
   ```

3. Open your browser to [http://localhost:7420](http://localhost:7420) and create your account.

4. Once you've verified it works, stop rui with `Ctrl+C`. We'll set it up as a background task next.

### Configuration

rui stores its configuration and database in `%APPDATA%\rui\` by default. For more details, see the [Configuration](/docs/configuration/environment) section.

## Create a Windows Task

To run rui in the background, we'll use **Task Scheduler**.

1. Press the **Windows key** and search for **Task Scheduler**.
2. Click **Create Basic Task** in the right sidebar.
3. **Name:** `rui` — optionally add a description like: *rui torrent management service*.
4. **Trigger:** Select **When the computer starts**.
5. **Action:** Select **Start a Program**.
   - **Program/script:** Browse to `C:\rui\rui.exe`
   - **Add arguments:** `serve`
   - **Start in:** `C:\rui`
6. Check **Open the Properties dialog** before finishing, then click **Finish**.

### Configure the task properties

In the Properties dialog:

- Under **General**, select **Run whether user is logged on or not**.
- Enter your Windows password when prompted.
- Optionally check **Run with highest privileges** if you encounter permission issues.

Click **OK** to save.

### Start the service

Right-click on **rui** in the Task Scheduler list and click **Run**.

:::tip
To restart the service, click **End** and then **Run** in the right sidebar of Task Scheduler.
:::

## Updating

rui has a built-in update command. You must stop the Task Scheduler job first, otherwise Windows will lock the executable and the update will fail.

1. Open **Task Scheduler**, right-click the **rui** task and click **End**.
2. Run the updater:
   ```powershell
   .\rui.exe update
   ```
3. Right-click the **rui** task again and click **Run** to restart it.

## Reverse Proxy (optional)

For remote access, it's recommended to run rui behind a reverse proxy like [Caddy](https://caddyserver.com/) or nginx for TLS and additional security.

See the [Base URL](/docs/configuration/base-url) section for reverse proxy configuration examples.

## Finishing Up

Once the task is running, rui will be available at [http://localhost:7420](http://localhost:7420). Add your qBittorrent instance(s) and start managing your torrents.
