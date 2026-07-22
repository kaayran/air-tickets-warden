# syntax=docker/dockerfile:1

# 1. Build the Mini App (Vite → web/dist).
FROM node:22-alpine AS web
WORKDIR /app/web
COPY web/package.json web/package-lock.json web/.npmrc ./
RUN npm ci
COPY web/ ./
RUN npm run build

# 2. Build the Go binary with the frontend embedded (go:embed web/dist).
FROM golang:1.26-alpine AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
COPY --from=web /app/web/dist ./web/dist
RUN CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags="-s -w" -o /warden ./cmd/warden

# 3. Minimal runtime image.
FROM gcr.io/distroless/static-debian12:nonroot
COPY --from=build /warden /warden
EXPOSE 8080 9090
USER nonroot:nonroot
ENTRYPOINT ["/warden"]
CMD ["run"]
