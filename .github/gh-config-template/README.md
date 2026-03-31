# Generate github actions from template

ytt -f ./gh_template.yml \
  -f [ytt-helpers.star](https://github.com/cloudfoundry/wg-app-platform-runtime-ci/blob/main/shared/helpers/ytt-helpers.star) \
  -f [index.yml](https://github.com/cloudfoundry/wg-app-platform-runtime-ci/blob/main/garden-runc-release/index.yml) \
  > ./workflows/tests-workflow.yml

## Supported jobs
- Template tests
- Lint repo
- Basic Verifications
- Unit and Integration tests (without DB)

### How to run

Workflow runs automatically on pull requests targeting the `develop` branch.
