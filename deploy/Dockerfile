FROM node:24-alpine AS web
WORKDIR /src/web
RUN corepack enable
COPY web/package.json web/pnpm-lock.yaml web/pnpm-workspace.yaml ./
RUN pnpm install --frozen-lockfile
COPY api /src/api
COPY web ./
RUN pnpm run generate && pnpm run build

FROM golang:1.25-alpine AS backend
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
COPY --from=web /src/web/dist ./web/dist
RUN go build -trimpath -ldflags="-s -w" -o /out/gkd ./cmd/gkd

FROM gcr.io/distroless/static-debian12:nonroot
COPY --from=backend /out/gkd /gkd
EXPOSE 8080
USER nonroot:nonroot
ENTRYPOINT ["/gkd"]
