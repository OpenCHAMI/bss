-- MIT License
--
-- (C) Copyright [2019-2021] Hewlett Packard Enterprise Development LP
--
-- Permission is hereby granted, free of charge, to any person obtaining a
-- copy of this software and associated documentation files (the "Software"),
-- to deal in the Software without restriction, including without limitation
-- the rights to use, copy, modify, merge, publish, distribute, sublicense,
-- and/or sell copies of the Software, and to permit persons to whom the
-- Software is furnished to do so, subject to the following conditions:
--
-- The above copyright notice and this permission notice shall be included
-- in all copies or substantial portions of the Software.
--
-- THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
-- IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
-- FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL
-- THE AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR
-- OTHER LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE,
-- ARISING FROM, OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR
-- OTHER DEALINGS IN THE SOFTWARE.

BEGIN;

--
-- nodes - Node identification information
--
CREATE TABLE IF NOT EXISTS nodes (
	id varchar PRIMARY KEY,
	boot_mac varchar,
	xname varchar,
	nid int
);

--
-- boot_group_assignments - Map nodes to boot_groups
--
CREATE TABLE IF NOT EXISTS boot_group_assignments (
	boot_group_id varchar,
	node_id varchar
);

--
-- boot_groups - Abstraction for a group of 1 or more nodes assigned
--               to a boot_config
--
CREATE TABLE IF NOT EXISTS boot_groups (
	id varchar PRIMARY KEY,
	boot_config_id varchar,
	name varchar,
	description varchar
);

--
-- boot_configs - Kernel URI and option initrd URI and kernel parameters
--
CREATE TABLE IF NOT EXISTS boot_configs (
	id varchar PRIMARY KEY,
	kernel_uri varchar,
	initrd_uri varchar,
	cmdline varchar
);

COMMIT;
