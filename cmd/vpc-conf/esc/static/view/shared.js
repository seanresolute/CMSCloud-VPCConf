const REGION = 0;
const ACCOUNT_ID = 1;
const VPC_ID = 2;

export class Shared {
    static InUseVPCToURL = (inUseVPC) => {
        const parts = inUseVPC.split(" - ")[0].split("/");
        return `accounts/${parts[ACCOUNT_ID]}/vpc/${parts[REGION]}/${parts[VPC_ID]}`;
    }
}
