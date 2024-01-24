-- Copyright © 2023 Triad National Security, LLC. All rights reserved.
--
-- This program was produced under U.S. Government contract 89233218CNA000001 for
-- Los Alamos National Laboratory (LANL), which is operated by Triad National
-- Security, LLC for the U.S. Department of Energy/National Nuclear Security
-- Administration. All rights in the program are reserved by Triad National
-- Security, LLC, and the U.S. Department of Energy/National Nuclear Security
-- Administration. The Government is granted for itself and others acting on its
-- behalf a nonexclusive, paid-up, irrevocable worldwide license in this material
-- to reproduce, prepare derivative works, distribute copies to the public,
-- perform publicly and display publicly, and to permit others to do so.

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
