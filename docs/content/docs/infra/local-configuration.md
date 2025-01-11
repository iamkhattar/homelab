---
title: "Local Configuration"
description: "Overview of local configuration."
weight: 2
toc: true
---

## Installing Terraform

Installing Terraform through Homebrew on macOS is a straightforward process. First, tap into HashiCorp's official
repository by running brew tap hashicorp/tap in the terminal. 

Install Terraform using Brew:

```bash
$ brew install hashicorp/tap/terraform
```

This method installs a signed binary that automatically updates with each new official release. 

Update Terraform by running:

```bash
$ brew update
$ brew upgrade hashicorp/tap/terraform
```

This approach ensures you always have access to the latest version of Terraform, making it an efficient and convenient
installation method for macOS users.

## Logging into Terraform Cloud

HCP Terraform runs Terraform operations and stores state remotely, so you can use Terraform without worrying about the
stability of your local machine, or the security of your state file.

To use HCP Terraform from the command line, you must log in. Logging in allows you to trigger remote plans and runs,
migrate state to the cloud, and perform other remote operations on configurations with HCP Terraform.

```bash
$ terraform login
Terraform will request an API token for app.terraform.io using your browser.

If login is successful, Terraform will store the token in plain text in
the following file for use by subsequent commands:
    /Users/redacted/.terraform.d/credentials.tfrc.json

Do you want to proceed?
  Only 'yes' will be accepted to confirm.

  Enter a value: yes
```

A browser window will automatically open to the HCP Terraform login screen. Enter a token name in the web UI, or leave
the default name, `terraform login`.

Next, click Create API token to generate the authentication token.

Save a copy of the token in a secure location. It provides access to your HCP Terraform organization. Terraform will also
store your token locally at the file path specified in the command output.

When the Terraform CLI prompts you, paste the user token exactly once into your terminal. Terraform will hide the token
for security when you paste it into your terminal. Press Enter to complete the authentication process.

```bash
Terraform must now open a web browser to the tokens page for app.terraform.io.

If a browser does not open this automatically, open the following URL to proceed:
    https://app.terraform.io/app/settings/tokens?source=terraform-login


---------------------------------------------------------------------------------

Generate a token using your browser, and copy-paste it into this prompt.

Terraform will store the token in plain text in the following file
for use by subsequent commands:
    /Users/redacted/.terraform.d/credentials.tfrc.json

Token for app.terraform.io:
  Enter a value:


Retrieved token for user redacted

Welcome to HCP Terraform!
```
