Stage 9 VM runs (2026-09-26, kes-node1 primary, kes-node2 standby).

- Most files: binary v2.0.0-alpha.1-24-g795daf4-dirty, i.e. before the
  locks fix in b17a63a. locks_node1_prepared_waiter.json therefore lacks the
  prepared transaction's own lock row; the text is unaffected.
- *_warn.* and session_*_idletxn_lowthr.*: binary v2.0.0-alpha.1-26-geeba1d0,
  taken past the default 10s lock wait (or with --idle-in-txn-warn 1).
