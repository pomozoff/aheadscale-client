import Foundation
import Security
import Darwin

public enum NativeEngineIdentity {
    public static let path = "/Library/PrivilegedHelperTools/ru.aheadscale.client.native-engine"
    public static let requirement = "anchor apple generic and certificate leaf[subject.OU] = \"Y24464MUL3\" and identifier \"ru.aheadscale.client.engine\""
    // Fixed, root-owned location; no PATH search, shell or executable supplied
    // through XPC/provider configuration. Verify the whole signature before run.
    public static func verify() throws {
        var info=stat()
        guard lstat(path,&info)==0,info.st_mode & mode_t(S_IFMT)==mode_t(S_IFREG),
              info.st_uid==0,info.st_mode & 0o022==0 else{throw NSError(domain:"ru.aheadscale.client.bridge",code:2)}
        var code:SecStaticCode?,req:SecRequirement?
        guard SecStaticCodeCreateWithPath(URL(fileURLWithPath:path) as CFURL,[],&code)==errSecSuccess,
              SecRequirementCreateWithString(requirement as CFString,[],&req)==errSecSuccess,
              let code,let req,
              SecStaticCodeCheckValidity(code,SecCSFlags(rawValue:kSecCSStrictValidate),req)==errSecSuccess else{throw NSError(domain:"ru.aheadscale.client.bridge",code:3)}
    }
}
