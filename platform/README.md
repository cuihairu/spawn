# Platform

以下均为规划，目录暂未落地。

- `infra`: Terraform/IaC、VPC、数据库、对象存储、K8s、Helm chart。
- `devops`: CI/CD pipeline、构建缓存、artifact、灰度与回滚策略、发布脚本。
- `observability`: 日志、指标、链路追踪、SLO、警报规则、事故回溯模板。

> 平台层提供统一的部署基线，要求与 services/apps 的配置项保持对齐，推荐在 docs/ 内同步编写 Runbook。
