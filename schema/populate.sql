-- -*-sql-*-
begin;

insert into lists (id, type, stem) VALUES ('spamcop', 'ip', 'bl.spamcop.net');
insert into customers (id) values ('marketo');
insert into customer_ips (customer, ip) VALUES ('marketo', '37.188.97.188/32');
insert into customer_ips (customer, ip) VALUES ('marketo', '72.3.185.0/24');
insert into customer_ips (customer, ip) VALUES ('marketo', '72.32.154.0/24');
insert into customer_ips (customer, ip) VALUES ('marketo', '72.32.217.0/24');
insert into customer_ips (customer, ip) VALUES ('marketo', '72.32.243.0/24');
insert into customer_ips (customer, ip) VALUES ('marketo', '94.236.119.0/26');
insert into customer_ips (customer, ip) VALUES ('marketo', '103.237.104.0/22');
insert into customer_ips (customer, ip) VALUES ('marketo', '130.248.172.0/23');
insert into customer_ips (customer, ip) VALUES ('marketo', '185.28.196.0/22');
insert into customer_ips (customer, ip) VALUES ('marketo', '192.28.128.0/18');
insert into customer_ips (customer, ip) VALUES ('marketo', '198.61.254.24/32');
insert into customer_ips (customer, ip) VALUES ('marketo', '198.61.254.29/32');
insert into customer_ips (customer, ip) VALUES ('marketo', '199.15.212.0/22');
commit;