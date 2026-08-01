#!/usr/bin/env bash

set -e
trap 'echo "Script interrupted."; exit 1' SIGINT

imageRepo=$1

service="be-classlist"

echo -e "🔧🔧🔧 Building and pushing image for $service 🔧🔧🔧 \n"


docker build -t "$service:v1" -f "./$service/Dockerfile" .

if [[ -n "$imageRepo" ]]; then
    echo -e "📦 Tagging and pushing $service to $imageRepo ...  \n"
    docker tag "$service:v1" "$imageRepo/$service:v1"
    docker push "$imageRepo/$service:v1"
else
    echo -e "No imageRepo provided, skipping tag & push for $service  \n"
fi
