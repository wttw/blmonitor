-- -*-sql-*-
begin;

select assert_schema_version(1);

drop table state;
drop table results;
drop table ips;
drop function ips_changed();
drop table customer_ips;
drop table customers;
drop table lists;
drop function assert_schema_version(integer);
drop table schema_version;
drop function schema_version_no_delete();

commit;