# rui

A fast, modern web interface for qBittorrent. Supports managing multiple qBittorrent instances from a single, lightweight application.

<div align="center">
  <img src="https://raw.githubusercontent.com/autogrr/.github/main/rui.png" alt="rui" width="100%" />
</div>

## Documentation

Full documentation available at **[getrui.com](https://getrui.com)**

## Quick Start

### Linux x86_64

```bash
# Download and extract the latest release
wget $(curl -s https://api.github.com/repos/autogrr/rui/releases/latest | grep browser_download_url | grep linux_x86_64 | cut -d\" -f4)
tar -C /usr/local/bin -xzf rui*.tar.gz

# Run
./rui serve
```

The web interface will be available at http://localhost:7476

### Docker

```bash
docker run -d \
  -p 7476:7476 \
  -v $(pwd)/config:/config \
  ghcr.io/autogrr/rui:latest
```

## Features

- **Single Binary**: No dependencies, just download and run
- **Multi-Instance Support**: Manage all your qBittorrent instances from one place
- **Fast & Responsive**: Optimized for performance with large torrent collections
- **Cross-Seed**: Automatically find and add matching torrents across trackers
- **Automations**: Rule-based torrent management with conditions and actions
- **Backups & Restore**: Scheduled snapshots with multiple restore modes
- **Reverse Proxy**: Transparent qBittorrent proxy for external apps

## Community

Join our community on [Discord](https://discord.autobrr.com/rui)!

## Support

- [GitHub Discussions](https://github.com/autogrr/rui/discussions/new/choose) - Feature requests and bug reports
- [GitHub Issues](https://github.com/autogrr/rui/issues) - Work in progress

## Contributing

Contributions are welcome. Note: this repo restricts pull request creation to **collaborators only**. Please start with a Discussion/Issue (or Discord) so we can coordinate changes.

## License

AGPL-1.0-or-later
