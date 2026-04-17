# Contributing to Cluster API Provider for Proxmox Virtual Environment

Thank you for considering contributing to the Cluster API Provider for Proxmox VE. We appreciate your time and effort to help make this project better. To ensure a smooth collaboration, please follow the guidelines below.

## Code of Conduct

This project follows the [CNCF Code of Conduct](https://github.com/cncf/foundation/blob/main/code-of-conduct.md). By participating, you are expected to uphold this code.

## How to Contribute

1. Fork the repository and create a new branch:
```sh
git clone https://github.com/your-username/cluster-api-provider-proxmox.git
cd cluster-api-provider-proxmox
git checkout -b feature-branch
```

2. Make your changes and commit them:

```shell
git add .
git commit -m "Your meaningful commit message"
```

3. Push your changes to your fork:

```shell
git push origin feature-branch
```

4. Open a pull request:

* Provide a clear and descriptive title for your changes.
* Include details about the changes and the problem it solves.
* Reference any relevant issues or pull requests.
* Keep changes complete and self-contained — partial implementations are not mergeable and stall review.
* Keep PRs focused and of reasonable size; smaller, focused changes are easier and faster to review and merge.
* A chain of dependent PRs is a good way to break up large contributions.

The ionos-cloud/cluster-api-provider-proxmox repo requires approval to run actions on external PRs.
This includes linters, unit (go) and e2e tests, scanners.

We use labels to control which e2e test contexts are run:

| Label | Behaviour |
|--|--|
| none | Run `Generic` tests only |
| https://github.com/ionos-cloud/cluster-api-provider-proxmox/labels/e2e%2Fnone | Do not run any e2e tests |
| https://github.com/ionos-cloud/cluster-api-provider-proxmox/labels/e2e%2Fflatcar | Add `Flatcar` tests |

The codeowner approving the run must make sure that the correct e2e labels are set.

## Development guide

For more in depth development documentation please check [our development guide](./docs/Development.md)

## Code Guidelines

* Write clear and concise code with meaningful variable and function names.
* Keep the code modular and well-documented.
* Ensure your code passes the existing tests.

## Testing

Make sure to run the existing tests before submitting your contribution.
If your contribution introduces new features, add appropriate tests to cover them.
Make sure that it's lint-free and that generated files are up to date.

```sh
make lint test verify
```

## AI-Assisted Contributions

All guidelines in this document apply to AI-assisted contributions. The rules in this section are in addition to those.

AI tools are welcome at any stage.

**Authorship**: The git commit author must be the person responsible for the contribution. Any agent that produced code must be attributed with a `Co-Authored-By:` trailer. The only exception is a commit that solely modifies an agent instructions file (e.g. `AGENTS.md`) where the agent may be the author and the person a co-author.

**Transparency**: AI agents must be clearly identified as such. We don't mind interacting with agents; we do mind agents masquerading as people. Do not have an agent interact on issues or pull requests while presenting as a human contributor.

**Human oversight**: The human contributor is responsible for the entire contribution and must oversee it end to end — code, tests, and description.

**Quality**: A non-draft pull request must represent completed work:
- Includes relevant tests covering the changes
- All tests and linters pass (`make lint test verify`)
- Correct attribution on all commits

Pull requests that violate these standards will be closed.

## Documentation
Ensure that your changes are reflected in the documentation. If you are introducing new features, update the documentation accordingly.

## Reporting Issues
If you encounter any issues or have suggestions for improvement, please open an issue on the GitHub repository.

## Thank You
Thank you for your contribution! Your efforts help make the Cluster API Provider for Proxmox VE better for everyone. We appreciate your dedication to the project and the CNCF community.
