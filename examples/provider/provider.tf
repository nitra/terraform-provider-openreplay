terraform {
  required_providers {
    openreplay = {
      source = "nitra/openreplay"
    }
  }
}

variable "openreplay_token" {
  type        = string
  sensitive   = true
  description = "Owner/admin OpenReplay dashboard JWT."
}

provider "openreplay" {
  endpoint = "https://replay.example.com/api"
  token    = var.openreplay_token
}
