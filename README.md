# Flexberry

CLI experimental para migrations, ORM e factories em projetos Go.

## Instalação

### Downloads

| Sistema | Arquitetura | Download |
|:---|:---|:---|
| Windows | x86-64 / AMD64 | [flexberry-windows-amd64.exe](https://github.com/PhelipeViana/flexberry/releases/download/v0.1.0-alpha.16/flexberry-windows-amd64.exe) |
| Linux | x86-64 / AMD64 | [flexberry-linux-amd64](https://github.com/PhelipeViana/flexberry/releases/download/v0.1.0-alpha.16/flexberry-linux-amd64) |
| Verificação | SHA-256 | [checksums.txt](https://github.com/PhelipeViana/flexberry/releases/download/v0.1.0-alpha.16/checksums.txt) |

Todos os arquivos e versões anteriores ficam disponíveis em
[GitHub Releases](https://github.com/PhelipeViana/flexberry/releases).

No Windows, renomeie o arquivo para `flexberry.exe`, coloque-o na raiz do
projeto, ao lado do `go.mod`, e execute:

```powershell
.\flexberry.exe
```

No Linux:

```bash
mv flexberry-linux-amd64 flexberry
chmod +x flexberry
./flexberry
```

O menu é carregado do arquivo público `config/menu.json`. Sem internet, o
executável usa um menu local seguro. O manifesto online pode apenas habilitar
comandos conhecidos; ele não executa scripts arbitrários.

Ao iniciar, o Flexberry também consulta os releases publicados. Quando encontra
uma versão mais nova, o menu normal é substituído pelas opções de baixar e
instalar automaticamente ou sair. No Windows, o download é validado com o
`checksums.txt`, o executável atual é preservado como `.old` e a versão nova é
aberta automaticamente.

## Comandos

```powershell
.\flexberry.exe config install
.\flexberry.exe config update
.\flexberry.exe config remove

.\flexberry.exe connection report

.\flexberry.exe migrate reload
.\flexberry.exe migrate run
.\flexberry.exe migrate run-all
.\flexberry.exe migrate fresh

.\flexberry.exe factory reload
.\flexberry.exe factory run

.\flexberry.exe orm reload
.\flexberry.exe orm run

.\flexberry.exe validate
.\flexberry.exe version
.\flexberry.exe self update
```

`connection report` abre e autentica em todas as conexões do
`connection.yaml`. O relatório mostra status, dialeto, schema, versão do banco,
tempo de resposta e destaca a conexão padrão.

`config install` cria somente configurações editáveis:

```text
internal/flexberry/
├── connection.yaml
├── migrate.yaml
├── orm.yaml
└── factory.yaml
```

- `connection.yaml`: ambientes e conexões.
- `migrate.yaml`: entidades monitoradas e histórico das migrations.
- `orm.yaml`: entidades de origem e destino do ORM.
- `factory.yaml`: destino, defaults e expressões das factories.

Os arquivos contêm comentários explicando cada campo.

## Fluxo

```powershell
.\flexberry.exe config install

# Revise os quatro arquivos YAML.

.\flexberry.exe migrate reload
.\flexberry.exe migrate run
.\flexberry.exe factory reload
.\flexberry.exe factory run
.\flexberry.exe orm reload
```

`migrate reload` compara as entidades com o snapshot monitorado e cria uma
migration Go declarativa, legível e nomeada por timestamp. Ela usa uma única
função como `migrate.CreateTable`, tabelas existentes com autocomplete em
`alias.` e builders como `migrate.Col("id").Integer().PrimaryKey()`. O catálogo
`dsl.gen.go` é atualizado ao planejar ou executar as migrations. `migrate run`
interpreta apenas essa DSL
segura e converte o plano
para o dialeto da conexão padrão e registra o checksum em `migrations_flex`.
Para migrar outro banco, altere `DB_DIALECT` no `.env`.

No mesmo Reload, o Flexberry reproduz todo o histórico das migrations e gera
`doc_entities.txt` na pasta configurada em `migrate.yaml > output.path`. O
arquivo mostra structs Go prontas para copiar, com tipos, nulabilidade e tags
`db`/`json` exatamente iguais às colunas da migration. Ele é documentação
gerada: não deve ser editado e nunca sobrescreve entidades do domínio.

Regras que não cabem nas tags convencionais `db` e `json` usam a tag
`migrate`. O tipo Go define o tipo da coluna e ponteiros são anuláveis:

```go
type TabelaTeste struct {
	ID        int64     `db:"id" json:"id" migrate:"primaryKey,autoIncrement"`
	Email     *string   `db:"email" json:"email" migrate:"size=150,unique"`
	Ativo     bool      `db:"ativo" json:"ativo" migrate:"default=true"`
	Valor     float64   `db:"valor" json:"valor" migrate:"precision=19,scale=4"`
	CriadoEm  time.Time `db:"criado_em" json:"criado_em" migrate:"index"`
	ClienteID int64     `db:"cliente_id" json:"cliente_id" migrate:"references=clientes.id"`
}
```

`primaryKey`, `autoIncrement`, `size`, `unique`, `default`, `precision`,
`scale`, `index`, `nullable` e `references=tabela.coluna` são validados pelo
Reload. `index` gera uma migration `CreateIndex` separada para manter a mesma
execução nos quatro dialetos. `AutoIncrement` nunca é presumido: precisa estar
declarado explicitamente na tag.

`migrate run-all` aplica as migrations em todas as conexões configuradas.
`migrate fresh` pede confirmação, remove somente as tabelas gerenciadas e o
histórico do banco padrão, e então reaplica todas as migrations do zero.

O package Go do ORM e das factories é inferido automaticamente pelo último
diretório configurado em `path`; não é necessário informar `package`.

`factory reload` sincroniza o ORM antes de gerar as factories. Regras do
`factory.yaml` funcionam como interceptadores e podem usar `COLUNA` ou
`TABELA.COLUNA`. A prioridade é: vínculo ORM, `exact` específico, `exact`
global, `contains`, expressão existente e fallback pelo tipo Go.

O scaffold inclui regras semânticas editáveis para nomes como `NOME`, `EMAIL`,
`CPF`, `CNPJ`, `CEP`, `TELEFONE`, `HASH`, `DESCRICAO`, `CIDADE` e `ATIVO`.
Os limites ficam visíveis no YAML e podem ser adaptados ao schema:

```yaml
expressions:
  exact:
    ATIVO: flexberry.FakeIntRange(index, 0, 1)
  contains:
    - pattern: EMAIL
      expression: flexberry.FakeEmail(index, 150)
```

Os helpers limitam texto por caracteres Unicode, preservam o sufixo
determinístico quando possível e aceitam limites opcionais:

```go
flexberry.FakeName(index, 100)
flexberry.FakeCPF(index, 14)
flexberry.FakeText(index, 255)
```

Relacionamentos `belongsTo` geram:

```go
flexberry.Vinculo("tabela_pai", "coluna_pai")
```

`factory run` cria um runner Go temporário dentro do projeto, testa a conexão,
limpa tabelas filhas antes das pais e insere pais antes dos filhos. O runner é
removido ao final.

## Desenvolvimento

```powershell
go fmt ./...
go vet ./...
go test ./...
go build -o flexberry.exe ./cmd/flexberry
```

## Publicação

Os executáveis são publicados automaticamente pelo workflow
`.github/workflows/release.yml`. Para publicar uma nova versão, crie e envie
uma tag semântica:

```powershell
git tag v0.1.0-alpha.16
git push origin v0.1.0-alpha.16
```

O GitHub Actions executa os testes, injeta `0.1.0-alpha.16` no binário, compila
Windows e Linux, gera `checksums.txt` e publica o release. Tags com hífen, como
as versões `alpha`, são marcadas automaticamente como pré-release.
