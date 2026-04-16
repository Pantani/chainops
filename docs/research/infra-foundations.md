# Infra Foundations Research for `bgorch`

## Escopo

Pesquisa orientada por documentação oficial para guiar arquitetura do `bgorch` em:

1. Dockerfile / Docker Build
2. Docker Compose
3. Ansible
4. Terraform
5. Kubernetes controllers/operators
6. Kubernetes stateful workloads
7. Go modules / organização de projeto em Go

## Fontes oficiais

### Docker
- [Dockerfile overview](https://docs.docker.com/build/concepts/dockerfile/)
- [Dockerfile reference](https://docs.docker.com/reference/dockerfile/)
- [Build best practices](https://docs.docker.com/build/building/best-practices/)
- [Compose file reference](https://docs.docker.com/compose/compose-file/)
- [Compose CLI reference](https://docs.docker.com/compose/reference/)
- [Compose application model](https://docs.docker.com/compose/intro/compose-application-model/)

### Ansible
- [Inventory guide](https://docs.ansible.com/ansible/latest/inventory_guide/index.html)
- [Check mode / diff mode](https://docs.ansible.com/projects/ansible/latest/playbook_guide/playbooks_checkmode.html)
- [Reusing playbooks and roles](https://docs.ansible.com/projects/ansible/latest/playbook_guide/playbooks_reuse.html)

### Terraform
- [terraform plan](https://developer.hashicorp.com/terraform/cli/commands/plan)
- [terraform apply](https://developer.hashicorp.com/terraform/cli/commands/apply)
- [Provisioners](https://developer.hashicorp.com/terraform/language/provisioners)
- [Backend configuration](https://developer.hashicorp.com/terraform/language/settings/backends/configuration)
- [S3 backend (locking/versioning guidance)](https://developer.hashicorp.com/terraform/language/backend/s3)
- [Style guide](https://developer.hashicorp.com/terraform/language/syntax/style)
- [Standard module structure](https://developer.hashicorp.com/terraform/language/modules/develop/structure)

### Kubernetes
- [Controllers](https://kubernetes.io/docs/concepts/architecture/controller/)
- [Operator pattern](https://kubernetes.io/docs/concepts/extend-kubernetes/operator)
- [Custom resources](https://kubernetes.io/docs/concepts/api-extension/custom-resources/)
- [StatefulSets](https://kubernetes.io/docs/concepts/workloads/controllers/statefulset/)
- [Persistent Volumes](https://kubernetes.io/docs/concepts/storage/persistent-volumes/)
- [Liveness/Readiness/Startup probes](https://kubernetes.io/docs/concepts/configuration/liveness-readiness-startup-probes/)

### Go
- [Managing module source](https://go.dev/doc/modules/managing-source)
- [Managing dependencies](https://go.dev/doc/modules/managing-dependencies)
- [Go Modules reference](https://go.dev/ref/mod)
- [Module release/version workflow](https://go.dev/doc/modules/release-workflow)

---

## 1) Dockerfile / Docker Build

### Modelo mental
Dockerfile descreve **build de imagem**; não é engine de operação contínua nem mecanismo de reconciliação de estado de cluster.

### Responsabilidade correta
- Empacotar runtime e dependências de processo.
- Produzir imagem reprodutível e pequena (multi-stage, base mínima, `.dockerignore`, pinning).
- Definir defaults de execução (entrypoint/cmd/env/ports).

### Boas práticas relevantes
- Multi-stage build e separação build/runtime.
- Base image confiável e enxuta.
- Evitar pacotes desnecessários.
- Rodar como usuário não-root quando possível.
- Build determinístico com versões explícitas.

### O que não empurrar para Dockerfile
- Lógica de bootstrap de ambiente externo.
- Orquestração entre processos/nós.
- Gestão de secrets (secret baked em imagem é anti-pattern).

### Impacto no `bgorch`
- Dockerfile fica como artefato de packaging por workload.
- Core não depende de container: workload pode ser binário host.
- Secrets entram via `SecretRef` em runtime, nunca no build.

---

## 2) Docker Compose

### Modelo mental
Compose é modelo declarativo para **aplicação multi-serviço local/simples**, com reconciliação via CLI (`docker compose up` reavalia config).

### Responsabilidade correta
- Definir services, networks, volumes, healthcheck e restart policy.
- Ambiente local/edge com estado simples.
- Overlay/merge de arquivos por ambiente.

### Boas práticas relevantes
- Arquivo Compose canônico e versionável.
- Named volumes para dados stateful.
- Healthchecks explícitos e restart policy consistente.
- Overrides por ambiente em arquivos separados.

### O que não empurrar para Compose
- Provisionamento de infraestrutura (VM, rede cloud, SG, storage externo).
- Controle avançado de rollouts multi-cluster.
- Política de reconciliação sofisticada por família de chain.

### Impacto no `bgorch`
- Compose é backend MVP de execução.
- `bgorch` gera Compose deterministicamente a partir do desired state.
- Core continua genérico, Compose só consome estado já resolvido.

---

## 3) Ansible

### Modelo mental
Ansible é automação procedural/declarativa para configuração remota de hosts com inventário e playbooks/roles.

### Responsabilidade correta
- Bootstrap de host.
- Distribuição de arquivos/templates.
- Setup de diretórios e serviços systemd.
- Execução idempotente de tarefas (com check/diff mode para validação).

### Boas práticas relevantes
- Inventário claro (estático/dinâmico).
- Reuso por roles.
- Check mode/diff mode em pipelines.
- Controle de variáveis e segredos com Vault/indireção.

### O que não empurrar para Ansible
- Control plane principal de reconciliação contínua do produto.
- Modelo de estado global cross-backend.
- Semântica de plugin de cadeia dentro de playbooks soltos.

### Impacto no `bgorch`
- Ansible entra como adapter de bootstrap/config host.
- Planejamento e diff continuam no engine do `bgorch`.
- `bgorch` decide *o que* aplicar; Ansible executa *como* no host.

---

## 4) Terraform

### Modelo mental
Terraform é IaC para **recursos de infraestrutura** com `plan`/`apply`, estado e locking em backend.

### Responsabilidade correta
- Provisionar infraestrutura base: rede, compute, storage, IAM, clusters.
- Gerenciar drift de recursos de infra via estado.
- Validar mudanças via `plan` antes de `apply`.

### Boas práticas relevantes
- Workflow de two-step plan/apply em automação.
- State remoto com lock e versionamento.
- Estrutura de módulos padrão e estilo consistente.
- `terraform validate` no CI.

### O que não empurrar para Terraform
- Operação runtime fina de processos blockchain.
- Bootstrap detalhado de daemon e lifecycle de serviço.
- Provisioners para tudo (HashiCorp trata provisioners como último recurso).

### Impacto no `bgorch`
- Terraform será adapter de provisionamento (não core).
- Core do `bgorch` mantém desired state operacional de node/workload.
- Boundary claro: Terraform entrega “substrato”; `bgorch` entrega operação.

---

## 5) Kubernetes controllers/operators

### Modelo mental
Kubernetes controllers seguem control-loop: observar estado atual e convergir para estado desejado. Operator é extensão desse padrão com CRDs + controller.

### Responsabilidade correta
- Reconciliação declarativa idempotente.
- Modelo `spec` (desired) + `status` (observed).
- Automação de rotinas operacionais para domínio específico.

### Boas práticas relevantes
- Separar responsabilidades entre controllers simples.
- Evitar acoplamento excessivo de dados de app ao API server.
- Usar CRDs para abstrações operacionais, não como banco de dados da aplicação.

### O que não empurrar para controller/operator
- Armazenamento arbitrário de dados de negócio.
- Lógica monolítica que mistura todas as responsabilidades.

### Impacto no `bgorch`
- Core adota semântica de reconciliação inspirada em controller.
- Modelo interno com desired/observed/diff/plan/apply/verify.
- Futuro backend Kubernetes pode mapear diretamente para StatefulSets/Services/PVCs.

---

## 6) Kubernetes para workloads stateful

### Modelo mental
Stateful workloads exigem identidade estável e storage persistente desacoplado do ciclo de vida de pod.

### Responsabilidade correta
- StatefulSet para identidade estável/ordem de rollout.
- PVC/PV/StorageClass para persistência e ciclo de vida de disco.
- Probes corretas (startup/readiness/liveness) com semântica apropriada.

### Boas práticas relevantes
- Não tratar stateful node como stateless deployment.
- Política de storage explícita (acesso, reclaim, classe).
- Startup probes para inicialização longa.
- Readiness separada de liveness para evitar cascata de restart.

### O que não empurrar para Kubernetes genérico
- Assumir que um único tipo de rollout serve para todos upgrades de chain.
- Ignorar requisitos de bootstrap/sync e lock de dados.

### Impacto no `bgorch`
- `StoragePolicy`, `SyncPolicy`, `UpgradePolicy` entram no domínio comum.
- Backend Kubernetes precisa traduzir essas políticas para primitives corretas.
- Modelo de health do core deve preservar startup/readiness/liveness semantics.

---

## 7) Go modules / organização de projeto em Go

### Modelo mental
Módulo Go é unidade versionável; código organizado em pacotes com fronteiras explícitas.

### Responsabilidade correta
- Core/CLI/plugins/backends como pacotes internos coesos.
- Dependências geridas via `go.mod`/`go.sum`.
- Evolução de API com versionamento de módulo e semver.

### Boas práticas relevantes
- Um módulo principal no root para reduzir fricção operacional.
- Uso de `internal/` para encapsular implementação.
- `cmd/` para binário(s) de entrada.
- `go mod tidy` e validação contínua em CI.

### O que não empurrar para layout Go
- Misturar API externa estável e detalhes internos sem fronteira.
- Multiplicar módulos cedo sem necessidade real.

### Impacto no `bgorch`
- Estrutura com `cmd/bgorch`, `internal/*`, `docs/*`, `examples/*`.
- API `v1alpha1` em pacote dedicado para evolução controlada.
- Registros de plugin/backend como ponto de extensão previsível.

---

## Design Implications for bgorch

| Tecnologia | Boa prática | Risco se usada errado | Decisão no bgorch |
|------------|-------------|-----------------------|---------------------|
| Dockerfile/Build | Multi-stage, imagem mínima, sem secrets embutidos | Imagens grandes, inseguras, build não reprodutível | Tratar Dockerfile só como packaging; secrets via `SecretRef` em runtime |
| Docker Compose | Services/volumes/networks/healthchecks declarativos | Virar pseudo-control-plane sem drift model | Backend Compose gera artefatos determinísticos a partir do core |
| Ansible | Roles idempotentes + check/diff mode | Virar engine principal e acoplar regras de chain | Adapter opcional de bootstrap/configuração host |
| Terraform | Plan/apply + state locking remoto | Empurrar runtime de chain para provisioners | Adapter de infra (VM/rede/storage), não runtime orchestrator |
| Kubernetes controllers/operators | Control loop e separação spec/status | Controller monolítico e acoplamento indevido | Core do `bgorch` segue reconciliação idempotente por design |
| Kubernetes stateful | StatefulSet + PVC + probes corretas | Tratar nó stateful como app stateless | Modelar `StoragePolicy`, `SyncPolicy`, `UpgradePolicy` no domínio comum |
| Go modules/layout | `cmd` + `internal` + módulo único inicial | Fronteiras fracas e evolução de API quebradiça | Go-first no core/CLI, API versionada e extensões por registry |
