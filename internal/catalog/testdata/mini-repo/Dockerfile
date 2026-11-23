# syntax=docker/dockerfile:1.10
ARG RUBY_VERSION=3.4.3
FROM registry.example.com/oss/docker.io/library/ruby:3.4.10-slim-bookworm AS base
FROM base AS builder
FROM node:20-alpine
FROM registry.example.com/base/utility@sha256:deadbeef
