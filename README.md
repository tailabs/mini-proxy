# Mini Proxy

A high-performance HTTP/HTTPS reverse proxy written in Go. This proxy forwards requests to a backend service specified by the `BACKEND_URL` environment variable and handles `X-Forwarded-*` headers correctly.

## Features

- High-performance seven-layer proxy
- Proper handling of `X-Forwarded-For`, `X-Forwarded-Proto`, and `X-Forwarded-Host` headers
- Configurable backend URL via environment variable
- Configurable port (defaults to 8080)

## Deployment

### Railway

You can deploy this proxy to Railway with one click:

[![Deploy on Railway](https://railway.app/button.svg)](https://railway.app/new/template?template=https%3A%2F%2Fgithub.com%2Ftailabs%2Fmini-proxy)

Or, you can deploy it manually by setting the `BACKEND_URL` environment variable in your Railway project.

## Environment Variables

- `BACKEND_URL` (required): The URL of the backend service to proxy requests to.
- `PORT` (optional): The port on which the proxy server will listen. Defaults to `8080`.

## Running Locally

1. Set the `BACKEND_URL` environment variable:
   ```bash
   export BACKEND_URL=https://your-backend-service.com
   ```

2. Run the proxy:
   ```bash
   go run main.go
   ```

   The proxy will start on port 8080 (or the port specified by the `PORT` environment variable).