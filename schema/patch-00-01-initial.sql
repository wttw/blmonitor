-- -*-sql-*-
begin;

CREATE FUNCTION schema_version_no_delete() RETURNS trigger
    LANGUAGE plpgsql
AS $$
begin
    raise exception 'Cannot delete schema version';
end;
$$;

create table schema_version (
    version integer not null
);

CREATE UNIQUE INDEX schema_version_one_row ON schema_version USING btree ((1));

CREATE TRIGGER schema_version_no_delete BEFORE DELETE ON schema_version FOR EACH ROW EXECUTE FUNCTION schema_version_no_delete();

CREATE FUNCTION assert_schema_version(expected integer) RETURNS boolean
    LANGUAGE plpgsql
AS $_$
declare
    ver schema_version%ROWTYPE;
begin
    begin
        select * into strict ver from schema_version;
    exception
        when NO_DATA_FOUND then
            raise exception 'Version not found in schema_version';
        when TOO_MANY_ROWS then
            raise exception 'schema_version malformed';
    end;
    if ver.version <> $1 then
        raise exception 'This patch can only be applied to schema version % not %', $1, ver.version;
    end if;
    return true;
end;
$_$;

insert into schema_version (version) values (1);

create table lists (
    id text primary key,
    type text not null check ( type in ( 'ip' ) ),
    stem text not null,
    testpos text not null default '127.0.0.2',
    testneg text not null default '127.0.0.1',
    throttle integer not null default 5,
    period integer not null default 7200
);

create table customers (
    id text primary key,
    lists text[]
);

insert into customers (id) values ('unknown');

create table customer_ips (
    id integer primary key generated always as identity,
    customer text not null references customers(id),
    ip cidr not null
);

create unique index ips_unique on customer_ips (customer, ip);

create table ips (
    ip inet primary key,
    stamp timestamptz not null default current_timestamp
);

create index ips_stamp_idx on ips(stamp);

create function ips_changed() returns trigger as $$
    begin
        perform pg_notify('ip', abbrev(NEW.ip));
        return NEW;
    end;
$$ language plpgsql;

create trigger ips_changed after insert or update on ips for each row execute function ips_changed();

create table results (
    id integer primary key generated always as identity,
    ip inet not null,
    customer text not null references customers(id),
    list text not null references lists(id),
    stamp timestamptz not null default current_timestamp,
    txt text not null,
    listed bool
);

create index results_ip on results(ip, customer, list, stamp);

create table state (
    id text primary key references lists(id),
    lastip text not null,
    stamp timestamptz not null
);

commit;