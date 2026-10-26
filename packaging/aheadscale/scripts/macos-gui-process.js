ObjC.import('Foundation');
// proc_pidpath reads the executable path from the kernel, independent of argv[0].
ObjC.bindFunction('proc_pidpath', ['int', ['int', 'void *', 'unsigned int']]);

function run(argv) {
    if (argv.length !== 2 || !/^[1-9][0-9]*$/.test(argv[0]) || Number(argv[0]) > 2147483647) {
        throw new Error('Expected a PID and the installed GUI executable path');
    }
    var data = $.NSMutableData.dataWithLength(4096);
    var length = $.proc_pidpath(Number(argv[0]), data.mutableBytes, 4096);
    if (length <= 0) return 'unavailable';
    data.setLength(length);
    return data.isEqualToData($(argv[1]).dataUsingEncoding($.NSUTF8StringEncoding)) ? 'match' : 'other';
}
