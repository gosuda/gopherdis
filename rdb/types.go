package rdb

const (
	// RDBVersion is the standard Redis RDB format version (Version 9 is universally compatible with Redis 5.x - 7.x).
	RDBVersion = 9

	// Opcode constants corresponding to Redis rdb.h
	OpcodeHashTemplate   = 242
	OpcodeKeyMeta        = 243
	OpcodeSlotInfo       = 244
	OpcodeFunction2      = 245
	OpcodeModuleAux      = 247
	OpcodeIdle           = 248
	OpcodeFreq           = 249
	OpcodeAux            = 250 // 0xFA - AUX field metadata
	OpcodeResizeDB       = 251 // 0xFB - Hash table size hint
	OpcodeExpireTimeMs   = 252 // 0xFC - Expire time in milliseconds (8 bytes uint64)
	OpcodeExpireTimeSec  = 253 // 0xFD - Expire time in seconds (4 bytes uint32)
	OpcodeSelectDB       = 254 // 0xFE - DB number selector
	OpcodeEOF            = 255 // 0xFF - End of RDB file

	// Value type opcodes
	TypeString           = 0
	TypeList             = 1
	TypeSet              = 2
	TypeZSet             = 3
	TypeHash             = 4
	TypeZSet2            = 5
	TypeModule           = 6
	TypeModule2          = 7
	TypeHashZipmap       = 9
	TypeListZiplist      = 10
	TypeSetIntset        = 11
	TypeZSetZiplist      = 12
	TypeHashZiplist      = 13
	TypeListQuicklist    = 14
	TypeStreamListpacks  = 15
	TypeHashListpack     = 16
	TypeZSetListpack     = 17
	TypeListQuicklist2   = 18

	// Special length encoding flags (2 most significant bits)
	Enc6BitLen           = 0    // 00xxxxxx
	Enc14BitLen          = 1    // 01xxxxxx
	Enc32BitLen          = 0x80 // 10000000
	Enc64BitLen          = 0x81 // 10000001
	EncVal               = 3    // 11xxxxxx (special encoding)

	// Special integer encodings
	EncInt8              = 0
	EncInt16             = 1
	EncInt32             = 2
	EncLZF               = 3
)
