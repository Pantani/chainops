# ADR 0003: Backend Model (Runtime Execution Adapters)

- Status: Accepted
- Date: 2026-04-16

## Context

`bgorch` deve operar em ambientes heterogêneos: local containers, VMs/bare metal, Kubernetes e integrações futuras com Terraform/Ansible.

## Decision

Definir backend como adapter de runtime com contrato mínimo:

- `ValidateTarget(spec)`
- `BuildDesired(ctx, spec, pluginOut)`

No MVP:
- backend implementado: `docker-compose`
- backend planejado próximo: `ssh-systemd`
- arquitetura já preparada para `kubernetes`, `terraform` (adapter infra), `ansible` (adapter bootstrap)

## Rationale

- Core calcula estado desejado e plano; backend traduz para artefatos/ações do alvo.
- Isola detalhes de execução (compose/systemd/k8s) do domínio de chain.

## Consequences

### Positivas
- Adição de runtime novo com impacto local.
- Possibilita parity de recursos com gaps explícitos por backend.
- Facilita testes por backend (golden de render + integração mínima).

### Negativas
- Nem todo backend suporta todas as capacidades (ex.: host mode em compose).
- Necessário manter matriz de compatibilidade plugin x backend.

## Boundaries

- **Terraform adapter**: provisionamento de infra (rede/compute/storage), não gestão de processo de chain.
- **Ansible adapter**: configuração/bootstrap de host, não control plane principal.

## Rejected alternatives

1. **Backend único obrigatório (Kubernetes-only ou Docker-only)**
   - Rejeitado: conflita com objetivo multi-runtime.
2. **Injetar detalhes de backend no core/planner**
   - Rejeitado: viola separação de responsabilidades e aumenta acoplamento.
