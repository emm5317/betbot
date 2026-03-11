# 007 Four-Sport Specialization

## Status

Accepted

## Decision

betbot is explicitly specialized to `MLB`, `NBA`, `NHL`, and `NFL`.

The project will not describe itself as a generic multi-sport system in near-term planning or architecture documentation.

## Why

- these four leagues have the strongest combination of market depth and public data quality
- they provide year-round operational coverage
- each sport has distinct, documented inefficiency patterns worth modeling deeply
- specialization makes the ETL, feature, and model layers more coherent

## Consequences

- sport-specific ETL workers are first-class concepts
- sport-aware scheduler and configuration behavior are required
- modeling and Kelly policy are not globally uniform
- soccer and open-ended sport expansion move out of near-term scope
