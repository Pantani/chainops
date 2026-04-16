# ADR 0001: Core Architecture (Declarative Reconciler in Go)

- Status: Accepted
- Date: 2026-04-16

## Context

`bgorch` precisa operar múltiplas famílias de blockchain, múltiplos processos por nó lógico e múltiplos runtimes (Compose, host/systemd, Kubernetes), sem acoplamento do core a uma chain específica.

Também é requisito explícito que o produto não seja apenas:
- provider Terraform,
- coleção de playbooks Ansible,
- wrapper raso de docker compose.

## Decision

Adotar core declarativo próprio em Go com os blocos:

1. **API versionada** (`v1alpha1`) para desired state.
2. **Validação + normalização** antes de qualquer render/plan.
3. **Planner** que compara desired state com snapshot observado/armazenado.
4. **Renderer** determinístico de artefatos de backend.
5. **Abstração de plugin** (família de chain) separada da abstração de backend (runtime/alvo).
6. **Estado local inicial** em arquivos JSON com possibilidade de evolução para backend transacional (SQLite/Postgres/etcd).

## Rationale

- Permite convergência idempotente e drift detection no próprio produto.
- Preserva separação entre semântica de chain e semântica de runtime.
- Mantém fronteiras claras para integrar Terraform/Ansible como adapters e não como núcleo.

## Consequences

### Positivas
- Arquitetura extensível por plugins/backends sem reescrever core.
- Contrato claro para `plan` first.
- Melhor testabilidade (validação, planner, render, golden).

### Negativas
- Mais código inicial do que acoplar direto em ferramentas prontas.
- Necessidade de manter compatibilidade de API/versionamento desde cedo.

## Rejected alternatives

1. **Terraform-first core**
   - Rejeitado: Terraform é excelente para provisionamento, mas inadequado como runtime reconciler de processos blockchain.
2. **Ansible-first core**
   - Rejeitado: playbooks não substituem um domínio declarativo com estado/plan/drift cross-backend.
3. **Compose-first product**
   - Rejeitado: prenderia o modelo ao runtime local e limitaria evolução para host/Kubernetes.
