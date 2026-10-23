#!/bin/sh
# Run in a configured OpenTofu directory; replace 42 with the dashboard project ID.
set -eu
tofu import openreplay_project.web 42
