---
sidebar_position: 3
title: Base URL
---

# Base URL Configuration

If you need to serve rui from a subdirectory (e.g., `https://example.com/rui/`), you can configure the base URL.

## Using Environment Variable

```bash
RUI__BASE_URL=/rui/ ./rui
```

## Using Configuration File

Edit your `config.toml`:

```toml
baseUrl = "/rui/"
```

## With Nginx Reverse Proxy

```nginx
# Redirect /rui to /rui/ for proper SPA routing
location = /rui {
    return 301 /rui/;
}

location /rui/ {
    proxy_pass http://localhost:7476/rui/;
    proxy_set_header Host $host;
    proxy_set_header X-Real-IP $remote_addr;
    proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
    proxy_set_header X-Forwarded-Proto $scheme;
}
```
