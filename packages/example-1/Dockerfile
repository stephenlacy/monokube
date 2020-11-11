FROM golang:1.14.11-alpine3.11 as build_image
ARG PACKAGE

WORKDIR /app
ADD main.go /app
RUN GOOS=linux GOARCH=386 CGO_ENABLED=0 go build -o "${PACKAGE}" main.go

FROM alpine:3
ARG PACKAGE

COPY --from=build_image /app/"${PACKAGE}" /app/
