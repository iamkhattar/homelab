---
title: "Overview"
description: "Overview of the infrastructure used by the Homelab."
weight: 1
toc: true
---

This repository's infrastructure is managed using Terraform, with state files securely stored in HashiCorp Terraform Cloud.
The deployment process is automated through GitHub Actions, which apply the Terraform configurations to the Hetzner account
upon merging a pull request into the main branch. To maintain code quality, standard linting checks are integrated into
the pull request workflow, ensuring the Terraform infrastructure adheres to best practices and conventions.

This setup leverages the benefits of Infrastructure as Code (IaC), allowing for consistent, version-controlled, and easily
reproducible infrastructure deployments. By utilizing Terraform Cloud for state management, the project gains improved
collaboration capabilities and enhanced security for state files. The automated GitHub Actions workflow streamlines the
deployment process, reducing manual intervention and potential human errors.
