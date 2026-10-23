# Combine with examples/provider/provider.tf and supply TF_VAR_openreplay_token.
resource "openreplay_project" "web" {
  name     = "Production"
  platform = "web"
}

output "project_key" {
  value = openreplay_project.web.project_key
}
