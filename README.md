# homelab

Welcome to my Homelab project, a dynamic and scalable infrastructure built on Hetzner Cloud using Terraform, Ansible,
and K3s. This setup is designed to host a variety of personal services and experimental projects, providing a flexible
and cost-effective platform for learning and development.

The infrastructure is provisioned using Terraform, which allows for easy scaling and management of Hetzner Cloud resources.
Ansible is employed for configuration management, ensuring consistent setup across all nodes and simplifying the installation
of K3s, a lightweight Kubernetes distribution. This combination of tools enables rapid deployment and modification of the
homelab environment, making it ideal for hosting both established services and new applications developed in my free time.
The use of K3s provides a robust container orchestration platform, allowing for efficient resource utilization and simplified
application deployment.

## Infrastructure configuration

This repository's infrastructure is managed using Terraform, with state files securely stored in HashiCorp Terraform Cloud.
The deployment process is automated through GitHub Actions, which apply the Terraform configurations to the Hetzner account
upon merging a pull request into the main branch. To maintain code quality, standard linting checks are integrated into
the pull request workflow, ensuring the Terraform infrastructure adheres to best practices and conventions.

This setup leverages the benefits of Infrastructure as Code (IaC), allowing for consistent, version-controlled, and easily
reproducible infrastructure deployments. By utilizing Terraform Cloud for state management, the project gains improved
collaboration capabilities and enhanced security for state files. The automated GitHub Actions workflow streamlines the
deployment process, reducing manual intervention and potential human errors.
