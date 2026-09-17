# Arquitetura do mesaposta-api — guia para a equipe

Este documento é o ponto de partida para quem está entrando no projeto agora.
Ele não repete o que já está bem documentado em outro lugar — aponta pra lá.
Se você é novo no time, leia este arquivo primeiro, do início ao fim, antes
de abrir qualquer módulo.

## 1. O que é este sistema

mesaposta-api é o backend de gestão de restaurante do MesaPosta: estoque,
compras, cardápio/receitas, clientes/fidelidade, comunidade (posts/curtidas)
e o fluxo de mesas/pedidos — incluindo um cardápio digital público acessado
por QR code, sem login.

É um **monólito modular multi-tenant**: um único binário Go
(`cmd/api`), um único banco Postgres compartilhado entre todos os
restaurantes (tenants), com isolamento de dados garantido em duas camadas
independentes — filtro explícito na aplicação **e** Row Level Security (RLS)
no próprio banco, para que um bug de aplicação nunca vaze dados entre
tenants sozinho.

Por dentro, o monólito é particionado em **módulos de domínio**
independentes (`internal/modules/<nome>/`), cada um dono dos seus próprios
dados e regras de negócio, se comunicando entre si só por contratos
explícitos e estreitos — nunca lendo o banco ou os tipos internos uns dos
outros diretamente.

## 2. Status atual — o que já existe

Todas as fases do plano de arquitetura original estão implementadas e
commitadas:

| Fase | Módulo | O que faz | Commit(s) |
|---|---|---|---|
| 0 | `internal/platform/**` | Shared kernel: tenant, auth, http, db+RLS, outbox, erros | `1f10074` |
| 1 | `operations` | Estoque, movimentações, fornecedores, pedidos de compra | `ae3da76`…`5a8c72d` |
| 2 | `catalog` | Produtos, receitas, descontos, promoção de receita a produto | `665cbf0` |
| 4 | `crm` | Clientes, saldo e histórico de pontos de fidelidade | `665cbf0` |
| 5 | `community` | Posts, comentários, curtidas (idempotentes) | `665cbf0` |
| — | shared kernel p/ ordering | Registro de tenants, resolução por slug, rate limiter, contrato de preço cross-module | `aa6cfa1` |
| 3 | `ordering` | Mesas, pedidos (gestão) + cardápio digital público via QR code | `3cf2550` |

**Observabilidade**: tracing (OTel/OTLP → Jaeger) e métricas (Prometheus,
incluindo p50/p95/p99 de latência HTTP por rota e saturação do pool de
conexões Postgres) cobrem o processo inteiro automaticamente, na
composition root (`internal/platform/observability`,
`internal/platform/httpx`) — nenhum módulo precisa de wiring próprio. Ver
[ADR-0007](adr/0007-tracing-metrics-rollout.md) e o dashboard Grafana
provisionado em [`deploy/grafana/`](../deploy/grafana). **Audit trail**
(quem fez o quê, quando, mudando o quê) é, ao contrário, opt-in por módulo
— hoje cobre `operations.ReceivePurchaseOrder`, `catalog.ApplyDiscount`,
`crm`'s ajustes de fidelidade e `ordering`'s transições de status de
pedido; ver [ADR-0006](adr/0006-audit-log.md) e `BACKLOG.md` item 7 para o
que falta e por quê.

O que **não** está implementado ainda (deliberadamente, ver §8): identidade
(`identity`/usuários como módulo próprio — hoje só existe autenticação, não
CRUD de usuário), idempotency keys, rate limiting fora da rota
pública de `ordering`, retenção/expurgo de PII, OpenAPI gerado, moderação
(delete post/comentário) em `community`. Tudo isso está catalogado com
justificativa em [`BACKLOG.md`](../BACKLOG.md).

## 3. Por onde começar a ler o código

Nesta ordem:

1. **Este arquivo** (visão geral).
2. [`internal/modules/README.md`](../internal/modules/README.md) — o
   template obrigatório de layout de um módulo, e as 6 regras de
   arquitetura (R1–R6) explicadas uma a uma, com o *porquê* de cada uma.
   **Leia isso antes de tocar em qualquer módulo** — é o documento que a
   fitness function (`architecture/rules.go`) torna executável.
3. Um módulo de referência completo: **`internal/modules/crm/`** é o mais
   simples e mais recente a seguir o padrão sem ambiguidade — bom primeiro
   exemplo de ponta a ponta (`domain/` → `application/` →
   `infrastructure/{http,postgres}/`).
4. **`internal/modules/ordering/`** se você for trabalhar em algo com
   superfície pública/não-autenticada — é o único módulo com dois adapters
   HTTP (`infrastructure/http/` autenticado +
   `infrastructure/httppublic/` público) e vale como referência de como
   isolar os dois modelos de confiança.
5. [`docs/adr/`](adr/) — as decisões de arquitetura registradas, na ordem:
   `0001` (por que o módulo Go vive na raiz do repo), `0002` (por que a
   fitness function é um teste Go, não uma ferramenta externa), `0003`
   (composition root e a fatia E2E de prova), `0004` (pipeline de CI e gate
   de cobertura), `0005` (rationale de isolamento transacional do
   `operations`), `0006` (audit log — port compartilhado + rollout
   faseado), `0007` (correlação trace/log, p99 por rota de verdade,
   gauges do pool de conexões).
6. [`README.md`](../README.md) (raiz) — como rodar localmente, `make
   check`, testes de integração, variáveis de ambiente.

## 4. Mapa dos módulos (bounded contexts)

```
                    internal/platform (shared kernel)
     tenant · auth · httpx · db+RLS · outbox · errs
                              │
        ┌──────────┬──────────┼──────────┬───────────┐
        │           │          │          │           │
    operations   catalog     crm      community    ordering
   (estoque,    (produtos,  (clientes, (posts,     (mesas,
    compras)     receitas)   fidelidade) curtidas)   pedidos +
                                                     cardápio
                                                     público)
        ▲           ▲
        └───────────┴── ordering lê preço de catalog via
                         catalog/contracts (primitivos apenas,
                         nunca catalog/domain.Money etc.)
```

Toda comunicação entre módulos é **in-process, via Go, nunca HTTP interno**
— e só através do pacote `contracts/` de quem está sendo consumido (regra
R3). Isso é o que preserva a opção de um dia extrair um módulo como
microsserviço de verdade sem reescrever regra de negócio nenhuma, só trocar
o adapter.

### O que cada módulo é dono de

| Módulo | Aggregate Roots | Domain Service | Rota autenticada | Rota pública |
|---|---|---|---|---|
| `operations` | `StockItem`, `PurchaseOrder`, `StockMovement`, `Transaction` | `ReceivePurchaseOrder` (cross-agregado, atômico) | `/v1/stock-items`, `/v1/purchase-orders` | — |
| `catalog` | `Product`, `Recipe` | — | `/v1/produtos`\*, `/v1/receitas`\* | — |
| `crm` | `Customer` (com `LoyaltyEntry` como entidade filha) | — | `/v1/customers`\* | — |
| `community` | `CommunityPost`, `Like` (agregados separados) | — | `/v1/posts`\* | — |
| `ordering` | `Table`, `Order`, `TableSession` | `OrderPricingService` (puro, sem I/O) | `/v1/mesas`, `/v1/pedidos` | `/public/v1/{tenantSlug}/**` |

\* nomes de rota exatos: confira `infrastructure/http/router.go` de cada
módulo — este mapa é pra orientação, não é o contrato definitivo.

### Por que `ordering` é diferente

É o único módulo com um adapter público sem autenticação — o cliente final
escaneia um QR code, não faz login. Duas peças de shared kernel foram
criadas especificamente para isso (não existiam antes da Fase 3):

- `internal/platform/auth/publictenant/` — resolve o tenant pelo **slug na
  URL** em vez de claim JWT, e injeta no `context.Context` exatamente como
  `auth.Authenticate` faria — o resto do módulo (repositórios, RLS,
  `UnitOfWork`) nunca sabe a diferença.
- `internal/platform/httpx/ratelimit.go` — rate limiter genérico em
  memória, montado nessa rota com chave `(IP do cliente, slug do tenant)`,
  **antes** da resolução de tenant na cadeia de middlewares (senão um
  atacante gasta um SELECT no banco por slug inventado, de graça).

Ver `internal/modules/ordering/infrastructure/httppublic/`'s doc comment
para o fluxo completo.

## 5. Multi-tenancy — como funciona na prática

Nunca é opcional, nunca é esquecível por acidente — é impedido de compilar
ou faz RLS bloquear silenciosamente:

1. Toda tabela de negócio tem `tenant_id UUID NOT NULL`, com `ENABLE
   ROW LEVEL SECURITY` + `FORCE ROW LEVEL SECURITY` e uma policy
   `tenant_isolation` padrão (`USING/WITH CHECK (tenant_id =
   current_setting('app.tenant_id', true)::uuid)`).
2. `internal/platform/db.Pool` **não expõe** `Query`/`Exec`/`QueryRow`
   diretos — só via `Pool.Scope(ctx)`, que falha se `ctx` não carregar um
   `tenant.TenantID` válido, e que seta `app.tenant_id` como
   `set_config(..., true)` **local à transação** antes de rodar qualquer
   SQL. Não tem como um repositório esquecer o filtro de tenant — ele não
   compila sem passar por esse caminho.
3. `tenant.TenantID` só existe dentro do código depois de passar por
   `tenant.Parse` — e só `internal/platform/auth/**` (e
   `internal/platform/auth/publictenant/`, para a rota pública) tem
   permissão de chamar isso (regra R6, verificada automaticamente).
4. Recurso que existe mas é de outro tenant sempre responde **404**, nunca
   403 — RLS torna a linha invisível, então o repositório já devolve
   "não encontrado" de forma indistinguível de "nunca existiu". Ver o doc
   comment de `httpx.NotFoundError`.
5. Exceção deliberada: a tabela `tenants` (migration `0008_tenants.sql`)
   **não** leva a policy `tenant_isolation` — ela é quem *estabelece*
   tenant, não pode depender de um tenant já resolvido. Ela tem uma policy
   de SELECT permissiva + `GRANT SELECT` limitado a colunas específicas.
   Está documentada assim no próprio SQL da migration — se você vir uma
   tabela sem `tenant_isolation`, essa é a única que deveria estar assim.
   A migration `0012_tenants_platform_fields.sql` adicionou grants de
   `INSERT`/`UPDATE` (também por coluna, excluindo `id`/`slug` do update)
   para a API de provisionamento de tenants (`internal/platform/platformadmin`)
   — autorização real fica inteiramente no token compartilhado dessa API,
   não no RLS. Ver [ADR-0008](adr/0008-platform-tenant-provisioning.md).

## 6. Fitness function de arquitetura (R1–R6)

Não é convenção de code review — é um teste Go (`go test
./architecture/...`) que falha o build se alguém violar uma fronteira.
Explicação completa de cada regra e o porquê:
[`internal/modules/README.md`](../internal/modules/README.md#the-architecture-rules-every-module-must-respect-r1-r6).

Resumo de uma linha cada, pra referência rápida:

- **R1** — `domain/` de nenhum módulo pode importar `net/http`,
  `database/sql`, pgx, chi, JWT, goose, argon2id, nem `internal/platform/**`
  fora de `{errs, tenant, outbox, audit}`.
- **R2** — `internal/platform/**` nunca importa `internal/modules/**`,
  exceto `internal/platform/app` (composition root).
- **R3** — módulo A só importa módulo B via `B/contracts`, nunca
  `B/domain`, `B/application` ou `B/infrastructure`.
- **R4** — `errs` e `tenant` só importam a standard library entre si.
- **R5** — `cmd/**` (e `internal/platform/app`) pode importar qualquer
  coisa — é o composition root, essa é a função dele.
- **R6** — só `internal/platform/auth/**` (e arquivos `_test.go`) podem
  chamar `tenant.Parse`.

Rode sempre com `-count=1` (`go test -count=1 ./architecture/...`) — a
suíte carrega o grafo de pacotes do disco em tempo de execução, e o build
cache do Go não rastreia isso como input do teste. Ver o passo a passo de
verificação manual no [`README.md`](../README.md#como-verificar-a-fitness-function-de-arquitetura)
da raiz se quiser ver a regra pegando uma violação de verdade.

## 7. Como adicionar um módulo novo (ou uma feature a um existente)

1. Leia [`internal/modules/README.md`](../internal/modules/README.md) do
   início ao fim — é o checklist oficial do primeiro commit de um módulo.
2. Escolha um módulo existente como referência de estrutura —
   `internal/modules/crm/` para o caso comum (só rota autenticada);
   `internal/modules/ordering/` se o novo módulo também vai precisar de
   superfície pública.
3. Ordem de implementação dentro do módulo, sempre: `domain/` (agregados +
   invariantes, testado sem I/O) → `application/` (casos de uso,
   orquestração, testado com fakes de repositório) → `infrastructure/`
   (adapters HTTP/Postgres, testado com `httptest`/testcontainers) — nessa
   ordem, nunca o inverso.
4. Regra de negócio mora no agregado (`domain/`), nunca solta num `if`
   dentro do use case. Ex.: "saldo insuficiente" é um método do agregado
   que retorna `errs.Conflict` (→ 409) e **deixa o estado do agregado
   intocado** em caso de rejeição — ver `StockItem.Withdraw` ou
   `Customer.RedeemPoints` como referência.
5. Todo agregado mutável tem `version` (lock otimista) — `UpdateWithVersion`
   no repositório retorna `errs.Conflict` se a versão não bater.
6. Rode `go test -count=1 ./architecture/...` a cada peça nova, não só no
   final — pega violação de fronteira cedo, antes de virar refatoração
   grande.
7. **Antes de considerar qualquer coisa pronta, rode `go build ./...`, `go
   vet ./...` e `go test ./...` de verdade** — nesta implementação, um
   módulo inteiro foi escrito sem isso e quebrou o build silenciosamente
   porque os handlers HTTP foram escritos contra uma API assumida, não a
   real. A lição: nunca considere uma tarefa pronta sem essas três saídas
   limpas na frente dos olhos.

## 8. O que fica para depois — ver `BACKLOG.md`

[`BACKLOG.md`](../BACKLOG.md) na raiz do repo cataloga toda melhoria
identificada durante a implementação que foi deliberadamente **não**
implementada agora, com justificativa de custo/benefício para cada uma —
idempotency keys, audit log, soft-delete, OpenAPI gerado, retenção de PII
do pedido público, rate limiting fora de `ordering`, entre outras. Antes de
propor uma dessas como trabalho novo, confira se já não está lá — a decisão
de adiar já tem um porquê registrado.

Duas decisões de produto que ficaram explicitamente em aberto (não são
dívida técnica, são escolha de negócio ainda não feita):

- Prazo de retenção de PII (nome/telefone) de pedidos feitos pelo cardápio
  público — hoje o sistema retém indefinidamente até uma política ser
  definida.
- Se um pedido público deve vincular automaticamente ao cadastro de
  fidelidade do CRM quando o telefone bate com um cliente existente — hoje
  não vincula.
