# ADR-0009: Financeiro (ERP) como serviço separado, não módulo interno

**Status**: Accepted
**Date**: 2026-09-14
**Author**: Manuelly Suzik (via Claude Code)
**Context**: `docs/rfcs/RFC-0001-modulo-erp.md` avaliou, sub-domínio por sub-domínio, se cada peça faltante do "ciclo ERP" deveria nascer como módulo interno (`internal/modules/<nome>/`) ou aplicação separada. Para o sub-domínio financeiro (contas a pagar/receber, fluxo de caixa), o RFC recomendou **módulo interno** (`internal/modules/finance`), citando ADR-0001 e a regra R3 (contratos in-process via `contracts/`) como justificativa: financeiro consome dados de `operations`/`ordering` intensamente e se beneficia de transação compartilhada e do RLS multi-tenant já existente.

Este ADR registra a reversão deliberada dessa recomendação específica: financeiro passa a ser construído como um serviço novo e independente, `mesaposta-erp-api`, em repositório próprio. O restante do RFC-0001 (fiscal via provedor terceirizado, RH/folha fora deste repositório) permanece válido e não é afetado por este ADR.

## Problema

Módulo interno era o *default* correto enquanto a integração com `finance` pudesse continuar sendo in-process (mesma transação, mesmo binário, `contracts/` compilado junto). A decisão do dono do produto foi priorizar, para o financeiro, um ciclo de deploy e evolução de contrato independentes do monólito `mesaposta-api` — o que exige o oposto do que R3 otimiza (acoplamento em compile-time dentro do mesmo binário): um contrato de rede versionado explicitamente entre dois processos. Construir isso como módulo interno e extrair depois teria sido possível, mas adiar a decisão não muda a arquitetura de comunicação que o financeiro vai precisar desde o primeiro commit.

## Decisão

### Novo serviço: `mesaposta-erp-api`

Repositório próprio, irmão de `mesaposta-api`/`mesaposta-frontend` em `~/projects/mesaposta/`, seguindo o mesmo padrão de ADR-0001 (raiz do repo = o serviço, `go.mod` na raiz, sem wrapper `services/api/`). Não importa `internal/**` de `mesaposta-api` — é um binário e um módulo Go completamente separados.

### Banco de dados: compartilhado, não módulo próprio

`mesaposta-erp-api` conecta na **mesma instância Postgres** e no mesmo conjunto de tenants que `mesaposta-api`, em vez de duplicar dados de tenant. Regras:

- `tenants` continua de escrita exclusiva do `mesaposta-api` (ver ADR-0008); `mesaposta-erp-api` nunca escreve nela e nunca faz o equivalente a `tenant.Parse` — só recebe um `tenant_id` já validado via claim do JWT.
- `mesaposta-erp-api` é dono de tabelas novas no mesmo banco físico (`bills`, `payments`, `receivables`, `cash_entries`), cada uma com `tenant_id` + policy `tenant_isolation`, replicando exatamente o padrão de RLS de `0004_operations.sql` e o mecanismo de GUC por transação de `internal/platform/db/scope.go` (`set_config('app.tenant_id', $1, true)`) — reimplementado no novo repositório, não importado.

### Autenticação

O JWT emitido pelo login do `mesaposta-api` (claims `tenant_id`/`role`/`sid`, ver `internal/platform/auth/token/claims.go`) é validado também pelo `mesaposta-erp-api`, via `JWT_SECRET` compartilhado entre os dois serviços — staff logado não precisa de um segundo login para usar o ERP. O canal de comunicação servidor-a-servidor (streaming de eventos financeiros, ver abaixo) usa uma credencial **separada** (mTLS ou token de serviço estático, no mesmo espírito do `PLATFORM_ADMIN_TOKEN` de ADR-0008) — nunca o JWT de sessão de um usuário.

### Contratos: gRPC + `mesaposta-erp-contracts`

Em vez de `mesaposta-api` publicar um client Go para o ERP consumir (inversão de dependência de um servidor sobre seu cliente), os `.proto` e o código Go gerado (client e server stubs, via `protoc-gen-go`/`protoc-gen-go-grpc`/`protoc-gen-grpc-gateway`) vivem num terceiro repositório, `mesaposta-erp-contracts`, versionado via tags semver e validado por `buf lint`/`buf breaking` em CI. `mesaposta-api` implementa o lado servidor; `mesaposta-erp-api` implementa o lado client e seu próprio servidor gRPC+grpc-gateway. Nenhum dos dois serviços depende do código interno do outro — ambos dependem só do contrato versionado.

`grpc-gateway` expõe simultaneamente REST/JSON e gRPC nativo a partir da mesma definição `.proto`, evitando manter DTO HTTP e mensagem gRPC como duas fontes de verdade para os endpoints de controle do financeiro (`RegisterSupplierBill`, `RegisterPayment`, `RegisterReceivable`, exportação CSV).

### Sincronização de eventos financeiros: streaming gRPC sobre o outbox existente, sem broker novo

`mesaposta-api` já tem uma tabela de outbox durável, `domain_events` (migração `0002_outbox.sql`, colunas `published_at`/`attempts`), que hoje não tem nenhum consumidor. Em vez de dar ao `mesaposta-erp-api` acesso de leitura direta a essa tabela (o que exigiria uma role Postgres com bypass de RLS, já que a policy `tenant_isolation` só libera um tenant por sessão), `mesaposta-api` expõe uma RPC de **server-streaming** (`SubscribeFinancialEvents`) que lê `domain_events` internamente e transmite pro `mesaposta-erp-api`, com um `resume_token` (último `id`/`occurred_at` consumido) enviado pelo client na reconexão para permitir replay.

Nenhum broker (Kafka/NATS) é introduzido nesta fase — decisão YAGNI, no mesmo espírito da recomendação do RFC-0001 contra motor fiscal próprio. Se fan-out para múltiplos consumidores justificar depois, o transporte interno pode trocar sem mudar o contrato exposto ao ERP, desde que o relay fique isolado atrás de uma interface própria desde o início.

O relay roda de forma assíncrona em relação à transação original de `ordering`/`operations` — uma indisponibilidade do `mesaposta-erp-api` nunca pode bloquear o fechamento de um pedido.

### Idempotência

Toda mensagem de evento relayada carrega o `id` (uuid) de `domain_events` como `source_event_id`. As tabelas financeiras do ERP aplicam `UNIQUE (tenant_id, source_event_id)`, sobrevivendo a reentrega at-least-once em caso de reconexão do stream — este é o mecanismo que substitui a garantia transacional que um módulo interno teria por estar na mesma transação do evento original.

### Auditoria

`mesaposta-erp-api` mantém sua própria trilha de auditoria (tabela e recorder próprios, mesmo padrão de ADR-0006), em vez de escrever na tabela de audit do `mesaposta-api` — cada serviço permanece dono dos seus próprios dados, inclusive de auditoria.

### Exportação LGPD

`mesaposta-erp-api` ganha um caso de uso de exportação em CSV escopado aos seus próprios dados financeiros (`bills`/`payments`/`receivables`/`cash_entries`), atendendo o direito de portabilidade (LGPD art. 18, VI). Portabilidade dos dados operacionais/CRM do `mesaposta-api` é um problema separado, fora do escopo deste ADR.

### Código de plataforma: não compartilhado

`db.Scope`/RLS, middleware de verificação de JWT, envelope de resposta HTTP e o carregador de config são reimplementados (não importados) em `mesaposta-erp-api` — cada repo permanece independentemente buildável/deployável, per ADR-0001. Só o contrato de rede (`mesaposta-erp-contracts`) é código compartilhado entre os dois serviços.

## Consequências

- Substitui, especificamente, a recomendação de "financeiro = módulo interno" do RFC-0001. O restante do RFC (fiscal terceirizado, RH fora do repo) permanece válido.
- Nova superfície operacional em `mesaposta-api`: um servidor gRPC (+ grpc-gateway para o lado REST, se algum endpoint do lado servidor precisar), health check padrão (`grpc.health.v1.Health`) espelhando `/v1/_probe`, propagação de trace OTEL através da fronteira gRPC (o `go.mod` já carrega `google.golang.org/grpc`/`grpc-gateway/v2`/`google.golang.org/protobuf` como dependências indiretas, provavelmente via o exportador OTLP/gRPC — nenhum conflito de versão esperado ao promovê-las a diretas).
- Nova coordenação de release entre três repositórios: uma mudança quebrando compatibilidade em `.proto` exige bump coordenado em `mesaposta-erp-contracts` e nos dois consumidores; mitigado por `buf breaking` bloqueando merge no repositório de contratos.
- Risco de lançamento duplicado (já citado no RFC-0001) passa a ser mitigado por chave de idempotência na tabela, não mais por transação compartilhada — é uma garantia estruturalmente mais fraca que a de um módulo interno, e precisa ser tratada como tal em toda a superfície de escrita do financeiro.
- `docs/ARCHITECTURE.md` e `BACKLOG.md` devem deixar de listar `finance` como módulo interno futuro e passar a referenciar `mesaposta-erp-api`/`mesaposta-erp-contracts` como serviços irmãos.
