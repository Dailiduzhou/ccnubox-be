#!/usr/bin/env bash

set -e
trap 'echo "Script interrupted."; exit 1' SIGINT

imageRepo=$1
service="be-class_v2"

echo "Building image for $service"
docker build -t "$service:v1" -f "./$service/Dockerfile" .

if [[ -n "$imageRepo" ]]; then
    echo "Tagging and pushing $service to $imageRepo"
    docker tag "$service:v1" "$imageRepo/$service:v1"
    docker push "$imageRepo/$service:v1"
else
    echo "No imageRepo provided, skipping tag and push"
fi
