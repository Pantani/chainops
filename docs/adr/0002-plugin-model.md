# ADR 0002: Plugin Model (Chain Family Extensions)

- Status: Accepted
- Date: 2026-04-16

## Context

O core precisa suportar qualquer blockchain, incluindo chains desconhecidas, sem vazamento de detalhes específicos para o schema comum.

## Decision

Definir interface de plugin com responsabilidades explícitas:

- `Validate(spec)`
- `Normalize(spec)`
- `Build(ctx, spec)` para produzir artefatos/metadata específicos
- `Capabilities()` para declarar suporte operacional

No schema, separar:

1. **Camada comum portátil**: nodePools, workloads, volumes, runtime, probes, políticas genéricas.
2. **Camada específica tipada**: `pluginConfig.<pluginName>`.

MVP implementa o plugin `generic-process`.

## Rationale

- Minimiza acoplamento do core com famílias de chain.
- Permite evolução incremental por plugin sem churn global na API.
- `generic-process` garante fallback para chain desconhecida com imagem/binário+comando+volumes+probes.

## Consequences

### Positivas
- Extensibilidade localizada.
- Validação especializada por família.
- Reuso de planner/backends entre plugins.

### Negativas
- Maior disciplina de versionamento de contratos de plugin.
- Necessidade de testes de compatibilidade plugin-core.

## Rejected alternatives

1. **Campos específicos de chain no schema raiz**
   - Rejeitado: degrada portabilidade e bloqueia evolução multi-chain.
2. **`map[string]any` sem contrato tipado**
   - Rejeitado: remove segurança de validação, dificulta tooling e gera erros tardios.
