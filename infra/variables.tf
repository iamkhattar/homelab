# Set the variable value in *.tfvars file
# or using the -var="hetzner_api_token=..." CLI option
variable "hetzner_cloud_api_token" {
  description = "Hetzner API token"
  sensitive   = true
  type        = string
}
