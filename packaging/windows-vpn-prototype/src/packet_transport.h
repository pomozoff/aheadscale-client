#pragma once
#include "ipc_probe.h"
#include <winrt/Windows.Foundation.Collections.h>
#include <memory>
#include <mutex>
#include <vector>
namespace aheadscale {
// Отдельный двунаправленный поток своего пакета. AppService хранит только
// управляющее соединение и передаёт одноразовые параметры подключения.
class PacketTransport {
public:
 static std::shared_ptr<PacketTransport> Listen();
 static std::shared_ptr<PacketTransport> Connect(winrt::Windows::Foundation::Collections::ValueSet const&, HANDLE stop);
 winrt::Windows::Foundation::Collections::ValueSet Offer() const;
 void Accept(HANDLE stop);
 std::vector<uint8_t> Read(HANDLE stop);
 void Write(std::vector<uint8_t> const&, HANDLE stop);
 void Cancel() noexcept;
private:
 Handle pipe;
 std::wstring tail, nonce;
 std::mutex writeMutex;
};
}
