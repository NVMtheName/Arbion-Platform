-- Fictional CI laboratory only. Deliberately NOT a production migration.
DO $$ BEGIN
  IF current_database() <> 'arbion_execution_sim' THEN
    RAISE EXCEPTION 'dedicated simulation test database required';
  END IF;
END $$;

CREATE SCHEMA execution_sim_lab;
CREATE TABLE execution_sim_lab.sessions (
  owner_id text NOT NULL CHECK (owner_id ~ '^[a-zA-Z0-9_-]{1,80}$'),
  account_id text NOT NULL CHECK (account_id ~ '^[a-zA-Z0-9_-]{1,80}$'),
  provider text NOT NULL CHECK (provider IN ('coinbase','schwab')),
  run_id text NOT NULL CHECK (run_id ~ '^[a-zA-Z0-9_-]{1,80}$'),
  genesis text NOT NULL CHECK (octet_length(genesis) BETWEEN 1 AND 65536),
  PRIMARY KEY (owner_id,account_id,provider,run_id),
  CHECK (((genesis::jsonb->>'sequence')::int=1
    AND genesis::jsonb->>'previous'=''
    AND genesis::jsonb->>'hash' ~ '^[a-f0-9]{64}$'
    AND NOT genesis::jsonb ? 'event'
    AND genesis::jsonb#>'{config,scope}'=jsonb_build_object('simulation_only',true,
      'owner',owner_id,'account',account_id,'provider',provider,'run',run_id)) IS TRUE)
);

CREATE TABLE execution_sim_lab.events (
  owner_id text NOT NULL,
  account_id text NOT NULL,
  provider text NOT NULL,
  run_id text NOT NULL,
  sequence integer NOT NULL CHECK (sequence BETWEEN 2 AND 10001),
  delivery_id text NOT NULL CHECK (delivery_id ~ '^[a-zA-Z0-9_-]{1,80}$'),
  record text NOT NULL CHECK (octet_length(record) BETWEEN 1 AND 65536),
  PRIMARY KEY (owner_id,account_id,provider,run_id,sequence),
  UNIQUE (owner_id,account_id,provider,run_id,delivery_id),
  FOREIGN KEY (owner_id,account_id,provider,run_id)
    REFERENCES execution_sim_lab.sessions ON DELETE RESTRICT,
  CHECK (((record::jsonb->>'sequence')::int=sequence
    AND record::jsonb->>'previous' ~ '^[a-f0-9]{64}$'
    AND record::jsonb->>'hash' ~ '^[a-f0-9]{64}$'
    AND NOT record::jsonb ? 'config'
    AND record::jsonb#>>'{event,id}'=delivery_id
    AND record::jsonb#>>'{event,kind}' IN ('ORDER_OPENED','SEND_RECORDED','ACKNOWLEDGED','FILL_SETTLED','CANCEL_REQUESTED','CANCEL_CONFIRMED','REJECTED','DEPOSIT_SETTLED','WITHDRAWAL_SETTLED')
    AND record::jsonb#>'{event,scope}'=jsonb_build_object('simulation_only',true,
      'owner',owner_id,'account',account_id,'provider',provider,'run',run_id)) IS TRUE)
);

CREATE FUNCTION execution_sim_lab.immutable() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
  RAISE EXCEPTION 'simulation evidence is immutable';
END $$;
CREATE TRIGGER immutable_session BEFORE UPDATE OR DELETE ON execution_sim_lab.sessions
  FOR EACH ROW EXECUTE FUNCTION execution_sim_lab.immutable();
CREATE TRIGGER immutable_events BEFORE UPDATE OR DELETE ON execution_sim_lab.events
  FOR EACH ROW EXECUTE FUNCTION execution_sim_lab.immutable();
CREATE TRIGGER no_session_truncate BEFORE TRUNCATE ON execution_sim_lab.sessions
  FOR EACH STATEMENT EXECUTE FUNCTION execution_sim_lab.immutable();
CREATE TRIGGER no_event_truncate BEFORE TRUNCATE ON execution_sim_lab.events
  FOR EACH STATEMENT EXECUTE FUNCTION execution_sim_lab.immutable();

CREATE FUNCTION execution_sim_lab.chain_insert() RETURNS trigger LANGUAGE plpgsql AS $$
DECLARE
  initial text;
  prior_sequence integer;
  prior_hash text;
  bytes_used bigint;
BEGIN
  SELECT genesis INTO STRICT initial FROM execution_sim_lab.sessions
    WHERE owner_id=NEW.owner_id AND account_id=NEW.account_id AND provider=NEW.provider AND run_id=NEW.run_id FOR UPDATE;
  SELECT sequence,record::jsonb->>'hash' INTO prior_sequence,prior_hash FROM execution_sim_lab.events
    WHERE owner_id=NEW.owner_id AND account_id=NEW.account_id AND provider=NEW.provider AND run_id=NEW.run_id
    ORDER BY sequence DESC LIMIT 1;
  prior_sequence := COALESCE(prior_sequence,1);
  prior_hash := COALESCE(prior_hash,initial::jsonb->>'hash');
  IF NEW.sequence <> prior_sequence+1 OR NEW.record::jsonb->>'previous' IS DISTINCT FROM prior_hash THEN
    RAISE EXCEPTION 'simulation chain does not extend current head';
  END IF;
  SELECT COALESCE(sum(octet_length(record)),0) INTO bytes_used FROM execution_sim_lab.events
    WHERE owner_id=NEW.owner_id AND account_id=NEW.account_id AND provider=NEW.provider AND run_id=NEW.run_id;
  IF bytes_used+octet_length(initial)+octet_length(NEW.record)>16777216 THEN
    RAISE EXCEPTION 'simulation journal capacity exceeded';
  END IF;
  RETURN NEW;
END $$;
CREATE TRIGGER validate_chain BEFORE INSERT ON execution_sim_lab.events
  FOR EACH ROW EXECUTE FUNCTION execution_sim_lab.chain_insert();
