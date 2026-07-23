# syntax=docker/dockerfile:1
FROM node:22-alpine AS web-build
WORKDIR /workspace/web
COPY web/package.json web/package-lock.json ./
RUN node -e "const fs=require('fs'); const p=JSON.parse(fs.readFileSync('package.json','utf8')); delete p.devDependencies['@sabishii/product-design-harness']; fs.writeFileSync('package.json', JSON.stringify(p, null, 2)+'\\n')" \
    && npm install --package-lock=false
COPY web/ ./
COPY schemas/ ../schemas/
RUN npm run build

FROM golang:1.23-alpine AS build
WORKDIR /app

# Allow downloading newer toolchain if needed
ENV GOTOOLCHAIN=auto

COPY go.mod go.sum ./
RUN go mod download
COPY . .
COPY --from=web-build /workspace/web/dist ./web/dist
RUN CGO_ENABLED=0 GOOS=linux go build -o codex-pool .

FROM alpine:3.20
RUN apk add --no-cache ca-certificates wget
RUN addgroup -S codex && adduser -S codex -G codex
WORKDIR /app
COPY --from=build /app/codex-pool /app/codex-pool
RUN mkdir -p /app/data /app/pool && chown -R codex:codex /app
USER codex
EXPOSE 8989
HEALTHCHECK --interval=30s --timeout=3s CMD wget -qO- http://127.0.0.1:8989/healthz || exit 1
ENTRYPOINT ["/app/codex-pool"]
