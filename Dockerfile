# Frontend Image
FROM nginx:alpine

RUN rm /etc/nginx/conf.d/default.conf

COPY nginx.conf /etc/nginx/nginx.conf

COPY delogger.html /usr/share/nginx/html/delogger.html

EXPOSE 80

CMD ["nginx", "-g", "daemon off;"]


# Backend Image
# FROM golang:1.23 AS builder

# WORKDIR /delogger

# COPY main.go go.mod go.sum ./

# RUN go mod download

# RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-s -w" -o bin main.go

# FROM alpine:latest

# WORKDIR /delogger

# COPY --from=builder /delogger/bin /delogger

# CMD ["/delogger/bin"]