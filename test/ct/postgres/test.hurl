POST http://hms-bss:27778/boot/v1/bootparameters
{
	"kernel": "https://testkerneluri1.tld",
	"initrd": "https://testinitrduri1.tld",
	"hosts": ["xname1", "xname2"],
	"params": "param1,param2"
}

GET http://hms-bss:27778/boot/v1/bootparameters

POST http://hms-bss:27778/boot/v1/bootparameters
{
	"kernel": "https://testkerneluri2.tld",
	"initrd": "https://testinitrduri2.tld",
	"nids": [1, 2],
	"params": "param3,param4"
}

GET http://hms-bss:27778/boot/v1/bootparameters

POST http://hms-bss:27778/boot/v1/bootparameters
{
	"kernel": "https://testkerneluri3.tld",
	"initrd": "https://testinitrduri3.tld",
	"nids": [1, 2],
	"macs": ["de:ad:be:ee:ee:ef", "c1:a5:51:f1:ab:1e"],
	"params": "param5,param6"
}

GET http://hms-bss:27778/boot/v1/bootparameters
