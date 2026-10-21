# Mermaid diagrams

## Request flow

```mermaid
flowchart LR
    Browser --> Vite
    Vite --> GoAPI["Go API & renderer"]
    click Browser href "https://example.com/" "Open example"
```

## Page load

```mermaid
sequenceDiagram
    participant User
    participant App
    participant Server
    User->>App: Open nested document
    App->>Server: GET /api/page
    Server-->>App: Rendered Markdown
```

## Document modes

```mermaid
stateDiagram-v2
    [*] --> Source
    Source --> Preview: toggle
    Preview --> Source: toggle
```

## Git graph

```mermaid
gitGraph
    commit id: "boot"
    branch experiment
    checkout experiment
    commit id: "feature"
```

## Syntax error fallback

```mermaid
flowchart LR
    A --> B
    B --> [broken
```
