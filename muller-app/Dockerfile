FROM golang:1.27.3 AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /muller-app . && mkdir /data

FROM gcr.io/distroless/static-debian12:nonroot
COPY --from=build /muller-app /muller-app
# The volume inherits this ownership, so the non-root user can write the database.
COPY --from=build --chown=65532:65532 /data /data
ENV MULLER_DB=/data/muller.db
VOLUME /data
EXPOSE 8080
ENTRYPOINT ["/muller-app"]
