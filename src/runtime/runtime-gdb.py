# Copyright 2010 The Go Authors. All rights reserved.
# Use of this source code is governed by a BSD-style
# license that can be found in the LICENSE file.

"""GDB Pretty printers and convenience functions for Go's runtime structures.

This script is loaded by GDB when it finds a .debug_gdb_scripts
section in the compiled binary. The [68]l linkers emit this with a
path to this file based on the path to the runtime package.
"""

# Known issues:
#    - pretty printing only works for the 'native' strings. E.g. 'type
#      foo string' will make foo a plain struct in the eyes of gdb,
#      circumventing the pretty print triggering.


from __future__ import print_function
import re
import sys
import gdb

print("Loading Go Runtime support.", file=sys.stderr)
#http://python3porting.com/differences.html
if sys.version > '3':
	xrange = range
# allow to manually reload while developing
goobjfile = gdb.current_objfile() or gdb.objfiles()[0]
goobjfile.pretty_printers = []

# G state (runtime2.go)

def read_runtime_const(varname, default):
  try:
    return int(gdb.parse_and_eval(varname))
  except Exception:
    return int(default)


G_IDLE = read_runtime_const("'runtime._Gidle'", 0)
G_RUNNABLE = read_runtime_const("'runtime._Grunnable'", 1)
G_RUNNING = read_runtime_const("'runtime._Grunning'", 2)
G_SYSCALL = read_runtime_const("'runtime._Gsyscall'", 3)
G_WAITING = read_runtime_const("'runtime._Gwaiting'", 4)
G_MORIBUND_UNUSED = read_runtime_const("'runtime._Gmoribund_unused'", 5)
G_DEAD = read_runtime_const("'runtime._Gdead'", 6)
G_ENQUEUE_UNUSED = read_runtime_const("'runtime._Genqueue_unused'", 7)
G_COPYSTACK = read_runtime_const("'runtime._Gcopystack'", 8)
G_DEADEXTRA = read_runtime_const("'runtime._Gdeadextra'", 10)
G_SCAN = read_runtime_const("'runtime._Gscan'", 0x1000)
G_SCANRUNNABLE = G_SCAN+G_RUNNABLE
G_SCANRUNNING = G_SCAN+G_RUNNING
G_SCANSYSCALL = G_SCAN+G_SYSCALL
G_SCANWAITING = G_SCAN+G_WAITING
G_SCANEXTRA = G_SCAN+G_DEADEXTRA

sts = {
    G_IDLE: 'idle',
    G_RUNNABLE: 'runnable',
    G_RUNNING: 'running',
    G_SYSCALL: 'syscall',
    G_WAITING: 'waiting',
    G_MORIBUND_UNUSED: 'moribund',
    G_DEAD: 'dead',
    G_ENQUEUE_UNUSED: 'enqueue',
    G_COPYSTACK: 'copystack',
    G_DEADEXTRA: 'extra',
    G_SCAN: 'scan',
    G_SCANRUNNABLE: 'runnable+s',
    G_SCANRUNNING: 'running+s',
    G_SCANSYSCALL: 'syscall+s',
    G_SCANWAITING: 'waiting+s',
    G_SCANEXTRA: 'extra+s',
}


#
#  Value wrappers
#

class SliceValue:
	"Wrapper for slice values."

	def __init__(self, val):
		self.val = val

	@property
	def len(self):
		return int(self.val['len'])

	@property
	def cap(self):
		return int(self.val['cap'])

	def __getitem__(self, i):
		if i < 0 or i >= self.len:
			raise IndexError(i)
		ptr = self.val["array"]
		return (ptr + i).dereference()


#
#  Pretty Printers
#

# The patterns for matching types are permissive because gdb 8.2 switched to matching on (we think) typedef names instead of C syntax names.
class StringTypePrinter:
	"Pretty print Go strings."

	pattern = re.compile(r'^(struct string( \*)?|string)$')

	def __init__(self, val):
		self.val = val

	def display_hint(self):
		return 'string'

	def to_string(self):
		l = int(self.val['len'])
		return self.val['str'].string("utf-8", "ignore", l)


class SliceTypePrinter:
	"Pretty print slices."

	pattern = re.compile(r'^(struct \[\]|\[\])')

	def __init__(self, val):
		self.val = val

	def display_hint(self):
		return 'array'

	def to_string(self):
		t = str(self.val.type)
		if (t.startswith("struct ")):
			return t[len("struct "):]
		return t

	def children(self):
		sval = SliceValue(self.val)
		if sval.len > sval.cap:
			return
		for idx, item in enumerate(sval):
			yield ('[{0}]'.format(idx), item)


class MapTypePrinter:
	"""Pretty print map[K]V types.

	Map-typed go variables are really pointers. dereference them in gdb
	to inspect their contents with this pretty printer.
	"""

	pattern = re.compile(r'^map\[.*\].*$')

	def __init__(self, val):
		self.val = val

	def display_hint(self):
		return 'map'

	def to_string(self):
		return str(self.val.type)

	def children(self):
		MapGroupSlots = 8 # see internal/abi:MapGroupSlots

		cnt = 0
		# Yield keys and elements in group.
		# group is a value of type *group[K,V]
		# The group layout depends on GOEXPERIMENT=mapsplitgroup:
		#   split:       group.keys[i] / group.elems[i]
		#   interleaved: group.slots[i].key / group.slots[i].elem
		# Detect which layout by checking for the 'keys' field.
		def group_slots(group):
			ctrl = group['ctrl']

			for i in xrange(MapGroupSlots):
				c = (ctrl >> (8*i)) & 0xff
				if (c & 0x80) != 0:
					# Empty or deleted
					continue

				# Full
				# The group layout depends on GOEXPERIMENT=mapsplitgroup:
				#   split:       group.keys[i] / group.elems[i]
				#   interleaved: group.slots[i].key / group.slots[i].elem
				try:
					yield str(cnt), group['slots'][i]['key']
					yield str(cnt+1), group['slots'][i]['elem']
				except gdb.error:
					yield str(cnt), group['keys'][i]
					yield str(cnt+1), group['elems'][i]

		# The linker DWARF generation
		# (cmd/link/internal/ld.(*dwctxt).synthesizemaptypes) records
		# dirPtr as a **table[K,V], but it may actually be two different types:
		#
		# For "full size" maps (dirLen > 0), dirPtr is actually a pointer to
		# variable length array *[dirLen]*table[K,V]. In other words, dirPtr +
		# dirLen are a deconstructed slice []*table[K,V].
		#
		# For "small" maps (dirLen <= 0), dirPtr is a pointer directly to a
		# single group *group[K,V] containing the map slots.
		#
		# N.B. array() takes an _inclusive_ upper bound.

		# table[K,V]
		table_type = self.val['dirPtr'].type.target().target()

		if self.val['dirLen'] <= 0:
			# Small map

			# We need to find the group type we'll cast to. Since dirPtr isn't
			# actually **table[K,V], we can't use the nice API of
			# obj['field'].type, as that actually wants to dereference obj.
			# Instead, search only via the type API.
			ptr_group_type = None
			for tf in table_type.fields():
				if tf.name != 'groups':
					continue
				groups_type = tf.type
				for gf in groups_type.fields():
					if gf.name != 'data':
						continue
					# *group[K,V]
					ptr_group_type = gf.type

			if ptr_group_type is None:
				raise TypeError("unable to find table[K,V].groups.data")

			# group = (*group[K,V])(dirPtr)
			group = self.val['dirPtr'].cast(ptr_group_type)

			yield from group_slots(group)

			return

		# Full size map.

		# *table[K,V]
		ptr_table_type = table_type.pointer()
		# [dirLen]*table[K,V]
		array_ptr_table_type = ptr_table_type.array(self.val['dirLen']-1)
		# *[dirLen]*table[K,V]
		ptr_array_ptr_table_type = array_ptr_table_type.pointer()
		# tables = (*[dirLen]*table[K,V])(dirPtr)
		tables = self.val['dirPtr'].cast(ptr_array_ptr_table_type)

		cnt = 0
		for t in xrange(self.val['dirLen']):
			table = tables[t]
			table = table.dereference()

			groups = table['groups']['data']
			length = table['groups']['lengthMask'] + 1

			# The linker DWARF generation
			# (cmd/link/internal/ld.(*dwctxt).synthesizemaptypes) records
			# groups.data as a *group[K,V], but it is actually a pointer to
			# variable length array *[length]group[K,V].
			#
			# N.B. array() takes an _inclusive_ upper bound.

			# group[K,V]
			group_type = groups.type.target()
			# [length]group[K,V]
			array_group_type = group_type.array(length-1)
			# *[length]group[K,V]
			ptr_array_group_type = array_group_type.pointer()
			# groups = (*[length]group[K,V])(groups.data)
			groups = groups.cast(ptr_array_group_type)
			groups = groups.dereference()

			for i in xrange(length):
				group = groups[i]
				yield from group_slots(group)


class ChanTypePrinter:
	"""Pretty print chan[T] types.

	Chan-typed go variables are really pointers. dereference them in gdb
	to inspect their contents with this pretty printer.
	"""

	pattern = re.compile(r'^chan ')

	def __init__(self, val):
		self.val = val

	def display_hint(self):
		return 'array'

	def to_string(self):
		return str(self.val.type)

	def children(self):
		# see chan.c chanbuf(). et is the type stolen from hchan<T>::recvq->first->elem
		et = [x.type for x in self.val['recvq']['first'].type.target().fields() if x.name == 'elem'][0]
		ptr = (self.val.address["buf"]).cast(et)
		for i in range(self.val["qcount"]):
			j = (self.val["recvx"] + i) % self.val["dataqsiz"]
			yield ('[{0}]'.format(i), (ptr + j).dereference())


def paramtypematch(t, pattern):
	return t.code == gdb.TYPE_CODE_TYPEDEF and str(t).startswith(".param") and pattern.match(str(t.target()))

#
#  Register all the *Printer classes above.
#

def makematcher(klass):
	def matcher(val):
		try:
			if klass.pattern.match(str(val.type)):
				return klass(val)
			elif paramtypematch(val.type, klass.pattern):
				return klass(val.cast(val.type.target()))
		except Exception:
			pass
	return matcher

goobjfile.pretty_printers.extend([makematcher(var) for var in vars().values() if hasattr(var, 'pattern')])
#
#  Utilities
#

def pc_to_int(pc):
	# python2 will not cast pc (type void*) to an int cleanly
	# instead python2 and python3 work with the hex string representation
	# of the void pointer which we can parse back into an int.
	# int(pc) will not work.
	try:
		# python3 / newer versions of gdb
		pc = int(pc)
	except gdb.error:
		# str(pc) can return things like
		# "0x429d6c <runtime.gopark+284>", so
		# chop at first space.
		pc = int(str(pc).split(None, 1)[0], 16)
	return pc


#
#  For reference, this is what we're trying to do:
#  eface: p *(*(struct 'runtime.rtype'*)'main.e'->type_->data)->string
#  iface: p *(*(struct 'runtime.rtype'*)'main.s'->tab->Type->data)->string
#
# interface types can't be recognized by their name, instead we check
# if they have the expected fields.  Unfortunately the mapping of
# fields to python attributes in gdb.py isn't complete: you can't test
# for presence other than by trapping.


def is_iface(val):
	try:
		return str(val['tab'].type) == "struct runtime.itab *" and str(val['data'].type) == "void *"
	except gdb.error:
		pass


def is_eface(val):
	try:
		return str(val['_type'].type) == "struct runtime._type *" and str(val['data'].type) == "void *"
	except gdb.error:
		pass


def lookup_type(name):
	try:
		return gdb.lookup_type(name)
	except gdb.error:
		pass
	try:
		return gdb.lookup_type('struct ' + name)
	except gdb.error:
		pass
	try:
		return gdb.lookup_type('struct ' + name[1:]).pointer()
	except gdb.error:
		pass


def iface_commontype(obj):
	if is_iface(obj):
		go_type_ptr = obj['tab']['_type']
	elif is_eface(obj):
		go_type_ptr = obj['_type']
	else:
		return

	return go_type_ptr.cast(gdb.lookup_type("struct reflect.rtype").pointer()).dereference()


def iface_dtype(obj):
	"Decode type of the data field of an eface or iface struct."
	# known issue: dtype_name decoded from runtime.rtype is "nested.Foo"
	# but the dwarf table lists it as "full/path/to/nested.Foo"

	dynamic_go_type = iface_commontype(obj)
	if dynamic_go_type is None:
		return
	dtype_name = dynamic_go_type['string'].dereference()['str'].string()

	dynamic_gdb_type = lookup_type(dtype_name)
	if dynamic_gdb_type is None:
		return

	type_size = int(dynamic_go_type['size'])
	uintptr_size = int(dynamic_go_type['size'].type.sizeof)	 # size is itself a uintptr
	if type_size > uintptr_size:
			dynamic_gdb_type = dynamic_gdb_type.pointer()

	return dynamic_gdb_type


def iface_dtype_name(obj):
	"Decode type name of the data field of an eface or iface struct."

	dynamic_go_type = iface_commontype(obj)
	if dynamic_go_type is None:
		return
	return dynamic_go_type['string'].dereference()['str'].string()


class IfacePrinter:
	"""Pretty print interface values

	Casts the data field to the appropriate dynamic type."""

	def __init__(self, val):
		self.val = val

	def display_hint(self):
		return 'string'

	def to_string(self):
		if self.val['data'] == 0:
			return 0x0
		try:
			dtype = iface_dtype(self.val)
		except Exception:
			return "<bad dynamic type>"

		if dtype is None:  # trouble looking up, print something reasonable
			return "({typename}){data}".format(
				typename=iface_dtype_name(self.val), data=self.val['data'])

		try:
			return self.val['data'].cast(dtype).dereference()
		except Exception:
			pass
		return self.val['data'].cast(dtype)


def ifacematcher(val):
	if is_iface(val) or is_eface(val):
		return IfacePrinter(val)

goobjfile.pretty_printers.append(ifacematcher)

#
#  Decimal floating point
#
# A decimal64 or decimal128 in memory is its IEEE 754 encoding XORed with
# the encoding of 0E+0, so that all bits clear is 0E+0, and the high word of
# a decimal128 is the one XORed (see d64load and d128load in the runtime).
# The encoding is BID except on linux/ppc64le, where it is DPD. The DWARF
# type is DW_ATE_decimal_float, which gdb would decode without undoing the
# XOR, and in the libdecnumber format gdb was built with rather than the
# target's, so DecimalPrinter decodes the bits itself. It applies wherever
# gdb prints a value of the type: a variable in a register or in memory, a
# struct field, an array, slice or map element, or a value reached through
# a pointer. The runtime records the encoding of 0E+0 in the constants read
# here; a binary without them gets no decimal printer.

DECIMAL_ZERO64 = read_runtime_const("'runtime._DecimalZero64'", 0)
DECIMAL_ZERO128_HI = read_runtime_const("'runtime._DecimalZero128Hi'", 0)

# The encodings of 0E+0, which tell the two formats apart.
BID_ZERO64 = 0x31c0000000000000
BID_ZERO128_HI = 0x3040000000000000
DPD_ZERO64 = 0x2238000000000000
DPD_ZERO128_HI = 0x2208000000000000


def decimal_encoding(zero64, zero128hi):
	"Return 'bid' or 'dpd' for the given encodings of 0E+0, or None."
	if (zero64, zero128hi) == (BID_ZERO64, BID_ZERO128_HI):
		return 'bid'
	if (zero64, zero128hi) == (DPD_ZERO64, DPD_ZERO128_HI):
		return 'dpd'
	return None


DECIMAL_ENCODING = decimal_encoding(DECIMAL_ZERO64, DECIMAL_ZERO128_HI)

# width: (coefficient bits of BID's first form, exponent bits, bias,
# largest coefficient, DPD declets)
DECIMAL_FORMATS = {
	64: (53, 10, 398, 10**16 - 1, 5),
	128: (113, 14, 6176, 10**34 - 1, 11),
}


def dpd_declet(d):
	"Return the integer in [0, 999] that the 10-bit DPD declet d encodes."
	p, q, r = d >> 9 & 1, d >> 8 & 1, d >> 7 & 1
	s, t, u = d >> 6 & 1, d >> 5 & 1, d >> 4 & 1
	v, w, x, y = d >> 3 & 1, d >> 2 & 1, d >> 1 & 1, d & 1
	pqr, stu, wxy = 4*p + 2*q + r, 4*s + 2*t + u, 4*w + 2*x + y
	if not v:
		a, b, c = pqr, stu, wxy
	elif (w, x) == (0, 0):
		a, b, c = pqr, stu, 8 + y
	elif (w, x) == (0, 1):
		a, b, c = pqr, 8 + u, 4*s + 2*t + y
	elif (w, x) == (1, 0):
		a, b, c = 8 + r, stu, 4*p + 2*q + y
	elif (s, t) == (0, 0):
		a, b, c = 8 + r, 8 + u, 4*p + 2*q + y
	elif (s, t) == (0, 1):
		a, b, c = 8 + r, 4*p + 2*q + u, 8 + y
	elif (s, t) == (1, 0):
		a, b, c = pqr, 8 + u, 8 + y
	else:
		a, b, c = 8 + r, 8 + u, 8 + y
	return 100*a + 10*b + c


def decimal_string(x, width, encoding):
	"""Return the character form of the decimal encoding x, as Go's fmt prints it.

	x is the encoding as an integer of width bits, 64 or 128, and encoding
	is 'bid' or 'dpd'. The form is IEEE 754 to-scientific-string, which
	keeps the quantum: 1.50 and 1.5 differ. Every NaN prints as NaN."""
	cbits, ebits, bias, maxcoeff, ndeclets = DECIMAL_FORMATS[width]
	neg = x >> (width - 1) & 1
	if x >> (width - 5) & 0xF == 0xF:
		if x >> (width - 6) & 1:
			return 'NaN'
		return '-Infinity' if neg else 'Infinity'
	if encoding == 'bid':
		if x >> (width - 3) & 3 == 3:
			# Second form: an implicit 100 above the stored bits.
			biased = x >> (cbits - 2) & ((1 << ebits) - 1)
			coeff = 1 << cbits | x & ((1 << (cbits - 2)) - 1)
		else:
			biased = x >> cbits & ((1 << ebits) - 1)
			coeff = x & ((1 << cbits) - 1)
		if coeff > maxcoeff:
			coeff = 0  # non-canonical: reads as zero
	else:
		# The combination field holds the two leading exponent bits and the
		# leading digit; the exponent continuation field the rest of the
		# exponent; the trailing significand field three digits per declet.
		comb = x >> (width - 6) & 0x1F
		if comb >> 3 == 3:
			lead, top = 8 + (comb & 1), comb >> 1 & 3
		else:
			lead, top = comb & 7, comb >> 3
		tbits = 10 * ndeclets
		cont = ebits - 2
		biased = top << cont | x >> tbits & ((1 << cont) - 1)
		coeff = lead
		for i in reversed(range(ndeclets)):
			coeff = coeff*1000 + dpd_declet(x >> (10*i) & 0x3FF)
	exp = biased - bias

	digs = str(coeff)
	n = len(digs)
	adj = exp + n - 1  # the power of ten of the leading digit
	sign = '-' if neg else ''
	if exp <= 0 and adj >= -6:
		if exp == 0:
			return sign + digs
		if n > -exp:
			return sign + digs[:n+exp] + '.' + digs[n+exp:]
		return sign + '0.' + '0'*(-exp - n) + digs
	if n > 1:
		digs = digs[0] + '.' + digs[1:]
	return '{0}{1}E{2}{3}'.format(sign, digs, '-' if adj < 0 else '+', abs(adj))


def decimal_memory_string(words, encoding, zero64, zero128hi):
	"""Return the character form of a decimal in its memory form.

	words is (x,) for a decimal64 and (lo, hi) for a decimal128, the
	words in memory order: a decimal128 is its low word followed by its
	high word on every target."""
	if len(words) == 1:
		return decimal_string(words[0] ^ zero64, 64, encoding)
	lo, hi = words
	return decimal_string((hi ^ zero128hi) << 64 | lo, 128, encoding)


DECIMAL_BYTE_ORDER = []


def decimal_byte_order():
	if not DECIMAL_BYTE_ORDER:
		big = 'big endian' in gdb.execute('show endian', to_string=True)
		DECIMAL_BYTE_ORDER.append('big' if big else 'little')
	return DECIMAL_BYTE_ORDER[0]


class DecimalPrinter:
	"Pretty print decimal64 and decimal128 values."

	def __init__(self, val):
		self.val = val

	def to_string(self):
		size = self.val.type.strip_typedefs().sizeof
		try:
			data = self.val.bytes  # gdb 14 and later
		except AttributeError:
			addr = self.val.address
			if addr is None:
				return '<decimal in a register: needs gdb 14 or later>'
			data = gdb.selected_inferior().read_memory(int(addr), size).tobytes()
		order = decimal_byte_order()
		words = tuple(int.from_bytes(data[i:i+8], order) for i in range(0, size, 8))
		return decimal_memory_string(words, DECIMAL_ENCODING, DECIMAL_ZERO64, DECIMAL_ZERO128_HI)


def decimalmatcher(val):
	if DECIMAL_ENCODING is None:
		return None
	t = val.type.strip_typedefs()
	if t.code != gdb.TYPE_CODE_DECFLOAT or t.sizeof not in (8, 16):
		return None
	# A C _Decimal64 or _Decimal128, from cgo, holds the plain encoding.
	if t.name is None or t.name.startswith('_Decimal'):
		return None
	return DecimalPrinter(val)

goobjfile.pretty_printers.append(decimalmatcher)

#
#  Convenience Functions
#


class GoLenFunc(gdb.Function):
	"Length of strings, slices, maps or channels"

	how = ((StringTypePrinter, 'len'), (SliceTypePrinter, 'len'), (MapTypePrinter, 'used'), (ChanTypePrinter, 'qcount'))

	def __init__(self):
		gdb.Function.__init__(self, "len")

	def invoke(self, obj):
		typename = str(obj.type)
		for klass, fld in self.how:
			if klass.pattern.match(typename) or paramtypematch(obj.type, klass.pattern):
				if klass == MapTypePrinter:
					fields = [f.name for f in self.val.type.strip_typedefs().target().fields()]
					if 'buckets' in fields:
						# Old maps.
						fld = 'count'

				return obj[fld]


class GoCapFunc(gdb.Function):
	"Capacity of slices or channels"

	how = ((SliceTypePrinter, 'cap'), (ChanTypePrinter, 'dataqsiz'))

	def __init__(self):
		gdb.Function.__init__(self, "cap")

	def invoke(self, obj):
		typename = str(obj.type)
		for klass, fld in self.how:
			if klass.pattern.match(typename) or paramtypematch(obj.type, klass.pattern):
				return obj[fld]


class DTypeFunc(gdb.Function):
	"""Cast Interface values to their dynamic type.

	For non-interface types this behaves as the identity operation.
	"""

	def __init__(self):
		gdb.Function.__init__(self, "dtype")

	def invoke(self, obj):
		try:
			return obj['data'].cast(iface_dtype(obj))
		except gdb.error:
			pass
		return obj

#
#  Commands
#

def linked_list(ptr, linkfield):
	while ptr:
		yield ptr
		ptr = ptr[linkfield]


class GoroutinesCmd(gdb.Command):
	"List all goroutines."

	def __init__(self):
		gdb.Command.__init__(self, "info goroutines", gdb.COMMAND_STACK, gdb.COMPLETE_NONE)

	def invoke(self, _arg, _from_tty):
		# args = gdb.string_to_argv(arg)
		vp = gdb.lookup_type('void').pointer()
		for ptr in SliceValue(gdb.parse_and_eval("'runtime.allgs'")):
			if ptr['atomicstatus']['value'] in [G_DEAD, G_DEADEXTRA]:
				continue
			s = ' '
			if ptr['m']:
				s = '*'
			pc = ptr['sched']['pc'].cast(vp)
			pc = pc_to_int(pc)
			blk = gdb.block_for_pc(pc)
			status = int(ptr['atomicstatus']['value'])
			st = sts.get(status, "unknown(%d)" % status)
			print(s, ptr['goid'], "{0:8s}".format(st), blk.function)


def find_goroutine(goid):
	"""
	find_goroutine attempts to find the goroutine identified by goid.
	It returns a tuple of gdb.Value's representing the stack pointer
	and program counter pointer for the goroutine.

	@param int goid

	@return tuple (gdb.Value, gdb.Value)
	"""
	vp = gdb.lookup_type('void').pointer()
	for ptr in SliceValue(gdb.parse_and_eval("'runtime.allgs'")):
		if ptr['atomicstatus']['value'] in [G_DEAD, G_DEADEXTRA]:
			continue
		if ptr['goid'] == goid:
			break
	else:
		return None, None
	# Get the goroutine's saved state.
	pc, sp = ptr['sched']['pc'], ptr['sched']['sp']
	status = ptr['atomicstatus']['value']&~G_SCAN
	# Goroutine is not running nor in syscall, so use the info in goroutine
	if status != G_RUNNING and status != G_SYSCALL:
		return pc.cast(vp), sp.cast(vp)

	# If the goroutine is in a syscall, use syscallpc/sp.
	pc, sp = ptr['syscallpc'], ptr['syscallsp']
	if sp != 0:
		return pc.cast(vp), sp.cast(vp)
	# Otherwise, the goroutine is running, so it doesn't have
	# saved scheduler state. Find G's OS thread.
	m = ptr['m']
	if m == 0:
		return None, None
	for thr in gdb.selected_inferior().threads():
		if thr.ptid[1] == m['procid']:
			break
	else:
		return None, None
	# Get scheduler state from the G's OS thread state.
	curthr = gdb.selected_thread()
	try:
		thr.switch()
		pc = gdb.parse_and_eval('$pc')
		sp = gdb.parse_and_eval('$sp')
	finally:
		curthr.switch()
	return pc.cast(vp), sp.cast(vp)


class GoroutineCmd(gdb.Command):
	"""Execute gdb command in the context of goroutine <goid>.

	Switch PC and SP to the ones in the goroutine's G structure,
	execute an arbitrary gdb command, and restore PC and SP.

	Usage: (gdb) goroutine <goid> <gdbcmd>

	You could pass "all" as <goid> to apply <gdbcmd> to all goroutines.

	For example: (gdb) goroutine all <gdbcmd>

	Note that it is ill-defined to modify state in the context of a goroutine.
	Restrict yourself to inspecting values.
	"""

	def __init__(self):
		gdb.Command.__init__(self, "goroutine", gdb.COMMAND_STACK, gdb.COMPLETE_NONE)

	def invoke(self, arg, _from_tty):
		goid_str, cmd = arg.split(None, 1)
		goids = []

		if goid_str == 'all':
			for ptr in SliceValue(gdb.parse_and_eval("'runtime.allgs'")):
				goids.append(int(ptr['goid']))
		else:
			goids = [int(gdb.parse_and_eval(goid_str))]

		for goid in goids:
			self.invoke_per_goid(goid, cmd)

	def invoke_per_goid(self, goid, cmd):
		pc, sp = find_goroutine(goid)
		if not pc:
			print("No such goroutine: ", goid)
			return
		pc = pc_to_int(pc)
		save_frame = gdb.selected_frame()
		gdb.parse_and_eval('$save_sp = $sp')
		gdb.parse_and_eval('$save_pc = $pc')
		# In GDB, assignments to sp must be done from the
		# top-most frame, so select frame 0 first.
		gdb.execute('select-frame 0')
		gdb.parse_and_eval('$sp = {0}'.format(str(sp)))
		gdb.parse_and_eval('$pc = {0}'.format(str(pc)))
		try:
			gdb.execute(cmd)
		finally:
			# In GDB, assignments to sp must be done from the
			# top-most frame, so select frame 0 first.
			gdb.execute('select-frame 0')
			gdb.parse_and_eval('$pc = $save_pc')
			gdb.parse_and_eval('$sp = $save_sp')
			save_frame.select()


class GoIfaceCmd(gdb.Command):
	"Print Static and dynamic interface types"

	def __init__(self):
		gdb.Command.__init__(self, "iface", gdb.COMMAND_DATA, gdb.COMPLETE_SYMBOL)

	def invoke(self, arg, _from_tty):
		for obj in gdb.string_to_argv(arg):
			try:
				#TODO fix quoting for qualified variable names
				obj = gdb.parse_and_eval(str(obj))
			except Exception as e:
				print("Can't parse ", obj, ": ", e)
				continue

			if obj['data'] == 0:
				dtype = "nil"
			else:
				dtype = iface_dtype(obj)

			if dtype is None:
				print("Not an interface: ", obj.type)
				continue

			print("{0}: {1}".format(obj.type, dtype))

# TODO: print interface's methods and dynamic type's func pointers thereof.
#rsc: "to find the number of entries in the itab's Fn field look at
# itab.inter->numMethods
# i am sure i have the names wrong but look at the interface type
# and its method count"
# so Itype will start with a commontype which has kind = interface

#
# Register all convenience functions and CLI commands
#
GoLenFunc()
GoCapFunc()
DTypeFunc()
GoroutinesCmd()
GoroutineCmd()
GoIfaceCmd()
