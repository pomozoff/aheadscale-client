#!/usr/bin/env python3
"""Проверка поставляемой policy на отдельной шине, без изменения системной шины."""
import os
from pathlib import Path
import subprocess
import sys
import tempfile
import time
from xml.sax.saxutils import escape

DEST = 'org.freedesktop.NetworkManager.aheadscale'
IFACE = 'org.freedesktop.NetworkManager.VPN.Plugin'
OBJECT = '/org/freedesktop/NetworkManager/VPN/Plugin'


def serve(address, ready, mutations):
    import dbus
    import dbus.service
    from dbus.mainloop.glib import DBusGMainLoop
    from gi.repository import GLib

    DBusGMainLoop(set_as_default=True)
    bus = dbus.bus.BusConnection(address)
    name = dbus.service.BusName(DEST, bus=bus, do_not_queue=True)

    class Fixture(dbus.service.Object):
        @dbus.service.method(IFACE, in_signature='t', out_signature='b')
        def SSOComplete(self, activation):
            return activation == 1

        @dbus.service.method(IFACE, in_signature='', out_signature='')
        def Disconnect(self):
            Path(mutations).write_text('called')

    fixture = Fixture(name, OBJECT)
    Path(ready).touch()
    GLib.MainLoop().run()


def stop(process):
    process.terminate()
    try:
        process.wait(timeout=5)
    except subprocess.TimeoutExpired:
        process.kill()
        process.wait()
        raise RuntimeError('Fixture не завершился штатно')


def main(policy):
    if os.geteuid() != 0:
        raise SystemExit('Нужен sudo только для изолированной шины с разными UID')
    # Временная шина использует только synthetic данные и отдельный Unix socket.
    with tempfile.TemporaryDirectory(prefix='aheadscale-dbus-policy-') as directory:
        work = Path(directory)
        work.chmod(0o755)
        installed = work / 'installed.conf'
        installed.write_bytes(Path(policy).read_bytes())
        installed.chmod(0o644)
        config = work / 'bus.conf'
        config.write_text('''<busconfig>
  <type>system</type>
  <listen>unix:path=%s/bus</listen>
  <auth>EXTERNAL</auth>
  <policy context="default">
    <allow user="*"/>
    <allow own="*"/>
    <allow send_destination="*"/>
    <allow receive_sender="*"/>
  </policy>
  <include>%s</include>
</busconfig>''' % (escape(directory), escape(str(installed))))
        bus = subprocess.Popen(['dbus-daemon', '--nofork', '--print-address=1', '--config-file=' + str(config)], stdout=subprocess.PIPE, text=True)
        try:
            address = bus.stdout.readline().strip()
            if not address.startswith('unix:'):
                raise RuntimeError('Отдельная шина не запустилась')
            ready, mutations = work / 'ready', work / 'mutations'
            service = subprocess.Popen(['/usr/bin/python3', __file__, '--serve', address, str(ready), str(mutations)])
            try:
                deadline = time.monotonic() + 10
                while not ready.exists():
                    if service.poll() is not None or time.monotonic() >= deadline:
                        raise RuntimeError('Сервис fixture не запустился')
                    time.sleep(.05)

                def call(member, *args, interface=IFACE, user='nobody', destination=DEST, object_path=OBJECT):
                    return subprocess.run(['runuser', '-u', user, '--', 'busctl', '--address=' + address, '--timeout=3', 'call', destination, object_path, interface, member, *args], capture_output=True, text=True, timeout=5)

                # Проверяем и успешное завершение, и чужую/старую попытку.
                for activation, expected in [('1', 'b true'), ('2', 'b false')]:
                    reply = call('SSOComplete', 't', activation)
                    assert reply.returncode == 0 and reply.stdout.strip() == expected, (activation, reply.stderr)
                # Запрет должен действовать для всех управляющих методов, включая будущие.
                for member in ['Connect', 'Disconnect', 'NewSecrets', 'NeedSecrets', 'SetConfig', 'SetIp4Config', 'SetIp6Config', 'FutureMutation']:
                    reply = call(member)
                    assert reply.returncode != 0 and 'Access denied' in reply.stderr, (member, reply.stdout, reply.stderr)
                for interface, member in [('org.example.Other', 'SSOComplete'), ('org.freedesktop.DBus.Properties', 'GetAll')]:
                    reply = call(member, interface=interface)
                    assert reply.returncode != 0 and 'Access denied' in reply.stderr, (interface, reply.stderr)
                reply = call('RequestName', 'su', DEST, '0', destination='org.freedesktop.DBus', object_path='/org/freedesktop/DBus', interface='org.freedesktop.DBus')
                assert reply.returncode != 0 and 'Access denied' in reply.stderr, reply.stderr
                assert not mutations.exists(), 'Управляющий вызов достиг сервиса'
                reply = call('Disconnect', user='root')
                assert reply.returncode == 0 and mutations.read_text() == 'called', reply.stderr
                print('PASS: обычный UID читает только SSOComplete; управление/владение запрещены; root сохранён')
            finally:
                stop(service)
        finally:
            stop(bus)


if __name__ == '__main__':
    if sys.argv[1] == '--serve':
        serve(*sys.argv[2:])
    else:
        main(sys.argv[1])
