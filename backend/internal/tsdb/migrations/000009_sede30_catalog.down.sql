-- Migration 009 rollback: remove the generic catalog señales it added.
-- (The demo plant / equipos / bindings this migration used to seed are no
-- longer created, so there is nothing else to roll back.)

DELETE FROM ssfv.tbl_senales
WHERE codigo_senal IN ('UB','UC','AP_A','AP_B','AP_C','RP_A','RP_B','RP_C');
