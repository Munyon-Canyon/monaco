ALTER TABLE evm_deposit_forwards ADD COLUMN fee_micros numeric(20, 0) CHECK (fee_micros >= 0);

CREATE INDEX evm_deposit_forwards_network_fee ON evm_deposit_forwards (network, arrived_at DESC, id DESC)
WHERE status = 'arrived' AND fee_micros IS NOT NULL;
