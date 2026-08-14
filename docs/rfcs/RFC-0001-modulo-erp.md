# RFC-0001 — Módulo ERP para o mesaposta-api

**Status**: Proposto
**Autor**: Manuelly Suzik (via Claude Code)
**Data**: 2026-08-14
**Escopo**: Avaliar a complexidade de introduzir funcionalidades de ERP no
mesaposta-api, identificar as mudanças/melhorias necessárias na arquitetura
atual, e decidir — por sub-domínio, não em bloco — se cada pedaço deve
nascer como módulo interno (`internal/modules/<nome>/`) ou como aplicação
separada.

## Resumo

O mesaposta-api já implementa, hoje, uma fatia relevante do que
tipicamente se chama "ERP" para um negócio de restaurante: estoque e
compras (`operations`), catálogo/receitas — que são, na prática, uma BOM
(*bill of materials*) — (`catalog`), CRM/fidelidade (`crm`) e
pedidos/mesas (`ordering`). "Introduzir um módulo ERP" não é, portanto,
adicionar uma peça do zero — é identificar o que falta para fechar o ciclo
ERP (principalmente **financeiro**, **fiscal** e **RH/folha**) e decidir,
para cada peça faltante, onde ela deve viver.

**Conclusão recomendada**: financeiro (contas a pagar/receber, fluxo de
caixa) nasce como módulo interno `finance`, seguindo o padrão R1–R6 já
estabelecido. Fiscal (NF-e/NFC-e/SPED) e RH/folha **não** devem ser
construídos dentro deste repositório — são domínios com ciclo de
compliance, risco regulatório e cardinalidade de dados completamente
ortogonais ao domínio de restaurante, e o próprio ADR 0001 já registra que
novos serviços "get their own repositories". A recomendação para esses dois
é: integrar com provedores terceirizados (fiscal) e/ou tratá-los como
aplicação separada (RH), não construir um motor fiscal ou de folha do zero
agora.

## Motivação

O pedido de negócio é genérico ("introduzir um módulo ERP"), mas o termo
"ERP" cobre domínios com naturezas muito diferentes entre si — estoque,
financeiro, fiscal, RH, ativos fixos, orçamento. Tratar isso como uma única
decisão binária (módulo vs. app separada) esconde que a resposta correta
não é a mesma para todos os sub-domínios. Este RFC existe para tornar essa
decisão explícita, sub-domínio por sub-domínio, antes de qualquer código
ser escrito — evitando tanto (a) inchar o monólito com domínios que não
deveriam compartilhar deploy/schema/ciclo de release, quanto (b) fragmentar
prematuramente em serviços separados algo que se beneficia de estar junto
(mesmo banco, mesmo RLS multi-tenant, comunicação in-process já pronta).

## Design Detalhado

### O que já existe hoje (equivalente a módulos ERP clássicos)

| Módulo atual | Equivalente ERP clássico | Cobre |
|---|---|---|
| `operations` | Inventory / Procurement | Itens de estoque, movimentações, fornecedores, pedidos de compra, recebimento |
| `catalog` | Product Master / BOM | Produtos, receitas (BOM), descontos |
| `crm` | CRM | Clientes, pontos de fidelidade |
| `ordering` | Sales / Order Management | Mesas, pedidos, cardápio público (QR code) |
| `community` | (sem equivalente ERP) | Diferencial de produto, não é ERP |

### O que falta para "fechar" o ciclo ERP, por sub-domínio

| Sub-domínio | O que cobriria | Complexidade | Recomendação | Por quê |
|---|---|---|---|---|
| **Financeiro** (contas a pagar/receber, fluxo de caixa) | Ligar `purchase_orders` recebidos a contas a pagar; ligar pedidos fechados (`ordering`) a contas a receber; caixa consolidado | **Média** | **Módulo interno** (`finance`) | Consome eventos/dados de `operations` e `ordering` intensamente; precisa de consistência transacional e do RLS multi-tenant já existentes; comunicação in-process via `contracts/` já é o padrão do repo (R3) |
| **Contabilidade gerencial** (razão simplificado, DRE/CMV gerencial) | Relatórios derivados do financeiro — não contabilidade societária | **Média** | Módulo interno, extensão de `finance`, fase posterior | Depende do financeiro existir primeiro; sem requisito regulatório próprio se ficar só no nível gerencial |
| **Fiscal** (NF-e/NFC-e, SPED, ICMS/PIS/COFINS/ISS) | Emissão de nota fiscal por venda, escrituração fiscal | **Muito alta** | **Não construir** — integrar com provedor terceirizado (ex.: categoria de serviços tipo eNotas/Focus NFe/NFe.io); se algum dia justificar operação própria, **aplicação separada** | Compliance regulatório muda por legislação estadual/federal, exige certificado digital A1/A3 por CNPJ, comunicação com SEFAZ (webservice + fila de contingência) — ciclo de mudança e superfície de risco nada parecidos com o domínio de restaurante |
| **RH / Folha de pagamento** | Funcionários, turnos, comissões, folha, eSocial | **Muito alta** | **Aplicação separada** (ou produto terceirizado integrado) | Domínio ortogonal — zero overlap de agregados com o core atual; compliance trabalhista (eSocial, INSS, FGTS) evolui independente do domínio de restaurante |
| **Ativos fixos / patrimônio** | Equipamentos de cozinha, depreciação | **Baixa** | Módulo interno, baixa prioridade | Poucas regras de negócio, nenhum overlap crítico de compliance |
| **Multi-filial / transferência entre estoques** | Transferência de itens entre unidades do mesmo tenant | **Média** | Extensão do módulo `operations` existente | `operations` já é dono de `StockItem`/`StockMovement`; é extensão natural do agregado existente, não um domínio novo |
| **Orçamento / planejamento** | Metas de custo, orçamento por período | **Média-baixa** | Módulo interno, extensão de `finance`, fase posterior | Depende do financeiro para ter dados reais para comparar contra orçamento |

### Por que "módulo interno" é o padrão-default, não "app separada"

O mesaposta-api já foi desenhado especificamente para isso: monólito
modular multi-tenant, módulos de domínio independentes comunicando-se só
via `contracts/` (regra R3), com a opção de extração futura para
microsserviço **preservada por design** sem reescrever regra de negócio —
ver `internal/modules/README.md` e a linha 96-100 de
`docs/ARCHITECTURE.md`. Fragmentar em serviços separados hoje, sem pressão
de escala real, contraria tanto o ADR 0001 (repo criado para um único
serviço) quanto o princípio YAGNI já adotado no projeto. Financeiro,
contabilidade gerencial, ativos fixos e orçamento são todos domínios que
**leem e escrevem dados que já vivem neste banco, deste multi-tenant** — não
há motivo técnico para separá-los.

### Por que fiscal e RH são a exceção

Dois sinais concretos, não só "parece complicado":

1. **Ciclo de compliance desacoplado do domínio de restaurante.** Legislação
   fiscal e trabalhista muda por conta própria, com prazos legais de
   adequação que não têm relação nenhuma com o roadmap de produto do
   mesaposta-api. Acoplar isso ao mesmo binário/deploy significa que uma
   mudança de legislação fiscal pode forçar um release fora de ciclo do
   sistema inteiro.
2. **O próprio ADR 0001 já decidiu isso.** "There is no multi-service
   monorepo requirement at this time, and no other service is expected to
   be added to this same repository — additional services (if any) would
   get their own repositories." Fiscal e RH, pela natureza acima, são
   exatamente esse caso.

Para fiscal especificamente, a recomendação vai além de "aplicação
separada" — é **não construir agora**: o mercado brasileiro tem provedores
maduros de emissão de NF-e/NFC-e via API que absorvem toda a complexidade
de SEFAZ/certificado/contingência. Construir isso do zero é reinventar uma
categoria de produto inteira sem necessidade.

## Alternativas Consideradas

1. **Construir tudo (financeiro, fiscal, RH) como módulos internos.**
   Rejeitado — fiscal e RH têm ciclo de mudança e superfície de risco
   desproporcionais ao resto do monólito; acoplar aumenta o *blast radius*
   de qualquer mudança regulatória.
2. **Extrair todo o sistema atual para microsserviços já de saída, junto
   com o ERP.** Rejeitado — prematuro, sem pressão de escala real, contraria
   ADR 0001 e o princípio YAGNI; a arquitetura atual já preserva a opção de
   extração futura sem esse custo antecipado.
3. **Construir motor fiscal próprio (emissão de NF-e in-house).** Rejeitado
   por ora — custo de compliance e manutenção contínua muito maior que
   integrar com um provedor terceirizado maduro; revisitar só se o volume de
   notas emitidas tornar o custo por nota do provedor proibitivo.

## Impacto e Riscos

- **Schema**: novas tabelas para `finance` (ex.: `bills`, `payments`,
  `cash_entries`), todas com `tenant_id` + policy `tenant_isolation`,
  seguindo o padrão de `0004_operations.sql`/`0010_audit_log.sql`. Nenhum
  impacto em tabelas existentes.
- **Risco de lançamento duplicado**: se a integração `operations`→`finance`
  (PO recebido → conta a pagar) ou `ordering`→`finance` (pedido fechado →
  conta a receber) falhar e for reexecutada, pode duplicar lançamento
  financeiro. Mitigação: usar o mesmo mecanismo de outbox
  (`internal/platform/outbox`) já usado pelos outros módulos, e o débito de
  idempotency keys já catalogado no `BACKLOG.md` (item 3) se tornou mais
  urgente com este módulo.
- **Risco operacional (fiscal)**: se a decisão for integrar provedor
  terceirizado, o sistema passa a depender de disponibilidade externa para
  emissão fiscal — precisa de fila de retry e alerta, não é fire-and-forget.
- **Auditoria**: seguindo o padrão já estabelecido (ADR 0006), toda
  movimentação financeira relevante (registro de conta, pagamento,
  estorno) deve ser wireada em `internal/platform/audit` desde o primeiro
  commit do módulo — não como débito técnico posterior, diferente do que
  aconteceu com `operations`/`catalog` originalmente.

## Plano de Adoção / Rollout

## Step 1 — Registrar ADR de posicionamento por sub-domínio

Formalizar, como ADR (`docs/adr/0007-...`), a decisão deste RFC:
financeiro como módulo interno `finance`; fiscal como integração com
provedor terceirizado (não construção própria); RH como aplicação separada
fora deste repositório. A decisão deve citar ADR 0001 e a regra R3 como
justificativa arquitetural.

## Step 2 — Desenhar o domínio do módulo `finance`

Modelar os agregados do módulo `finance` (`Bill`/conta a pagar,
`Receivable`/conta a receber, `Payment`, `CashEntry`) e suas invariantes
(ex.: uma conta paga não pode ser reaberta sem estorno explícito),
seguindo o layout obrigatório de `internal/modules/README.md`
(`domain/` → `application/` → `infrastructure/{http,postgres}` →
`contracts/`).

## Step 3 — Implementar a camada `domain/` do módulo `finance`

Implementar `Bill`, `Receivable`, `Payment`, `CashEntry` e os value objects
necessários (ex.: reaproveitar `Money` no padrão já usado por
`operations`/`catalog`), com lock otimista (`version`) igual aos outros
agregados mutáveis do projeto, testado sem I/O.

## Step 4 — Implementar a camada `application/` do módulo `finance`

Implementar os casos de uso `RegisterSupplierBill`, `RegisterPayment`,
`RegisterReceivable`, `ReconcileCashEntry`, orquestrando `domain/` e os
ports de repositório via fakes, seguindo o padrão de teste já usado em
`crm/application`.

## Step 5 — Implementar `infrastructure/{postgres,http}` do módulo `finance`

Criar a migration (`tenant_id` + RLS `tenant_isolation`, seguindo
`0004_operations.sql`), os repositórios Postgres via
`internal/platform/db.Pool.Scope`, e os handlers HTTP autenticados
(`/v1/bills`, `/v1/payments`) respondendo via `httpx.Envelope`, com
`internal/platform/audit` wireado desde o primeiro commit (não como débito
posterior). Fora de escopo: emissão de nota fiscal (NF-e/NFC-e) — ver
Step 7.

## Step 6 — Integrar `finance` com `operations` e `ordering` via `contracts/`

Expor em `operations/contracts` e `ordering/contracts` os eventos/portas
mínimas necessárias (ex.: PO recebido → gera conta a pagar; pedido fechado
→ gera conta a receber), publicados via `internal/platform/outbox` para
evitar lançamento duplicado em caso de reexecução, respeitando R3 (nunca
importar `domain`/`application`/`infrastructure` de outro módulo
diretamente). Fora de escopo: conciliação bancária automática via Open
Finance.

## Step 7 — Pesquisar e decidir o caminho fiscal (NF-e/NFC-e/SPED)

Levantar 2–3 provedores de emissão fiscal via API adequados ao porte de
restaurante multi-tenant, comparar custo por nota/mensalidade, cobertura
de NFC-e (varejo/consumidor final) vs. NF-e, e requisitos de certificado
digital por tenant. Produzir recomendação final de provedor (ou confirmar
que nenhum resolve o caso de uso e motor próprio precisa ser reavaliado).

## Step 8 — Atualizar documentação e backlog

Atualizar `docs/ARCHITECTURE.md` (mapa de módulos, tabela de aggregate
roots) e `BACKLOG.md` com o novo módulo `finance` e o item deferido
"emissão fiscal — decisão de provedor" apontando para o resultado do Step
7, seguindo o mesmo padrão de registro já usado para os módulos
existentes.

## Perguntas em Aberto

- Hoje, 1 tenant = 1 restaurante, ou já existe (ou está no roadmap)
  suporte a múltiplas filiais dentro do mesmo tenant? Isso muda o desenho
  de "multi-filial/transferência entre estoques".
- Os restaurantes-cliente já usam um escritório de contabilidade externo?
  Se sim, o módulo `finance` pode mirar em **exportação** (CSV/OFX) para o
  contador em vez de virar contabilidade societária completa.
- Qual o apetite de investimento mensal em provedor fiscal terceirizado
  (custo por nota emitida), antes de descartar de vez a opção de motor
  fiscal próprio?
- RH/folha entra no roadmap de produto do mesaposta-api de fato, ou é
  fora do escopo de produto (cliente usa outra ferramenta)? Isso decide se
  vale a pena sequer prototipar uma aplicação separada para isso agora.
