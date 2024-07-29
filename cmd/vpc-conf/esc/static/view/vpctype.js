import { html } from '../lit-html/lit-html.js';

// from the VPCType enumerated constants in models.go
const vpcTypeV1 = 0;
const vpcTypeLegacy = 1;
const vpcTypeException = 2;
const vpcTypeV1Firewall = 3;
const vpcTypeMigratingV1ToV1Firewall = 4;
const vpcTypeMigratingV1FirewallToV1 = 5;

export const VPCType = {
     canAddFirewall: function(vpcType) {
        return vpcType === vpcTypeV1 || this.isFirewallMigrationType(vpcType);
    },
     canRemoveFirewall: function(vpcType) {
        return vpcType === vpcTypeV1Firewall || this.isFirewallMigrationType(vpcType);   
    },
     isFirewallMigrationType: function(vpcType) {
        return vpcType === vpcTypeMigratingV1ToV1Firewall || vpcType === vpcTypeMigratingV1FirewallToV1;
    },
    // Return a colored text abbreviation with a tooltip to keep list views uncluttered
     getStyled: function(vpcType) {
        let template = html`<span class="tooltip ds-u-color--gray" data-tooltip="Unmanaged">—</span>`;
    
        if (vpcType === vpcTypeV1) {
            template = html`<span class="tooltip ds-u-color--success" data-tooltip="Version 1">V1</span>`;
        } else if (vpcType === vpcTypeLegacy) {
            template = html`<span class="tooltip ds-u-color--primary-darker" data-tooltip="Legacy">LEG</span>`;
        } else if (vpcType === vpcTypeException) {
            template = html`<span class="tooltip ds-u-color--error-dark" data-tooltip="Exception">EXC</span>`;
        } else if (vpcType === vpcTypeV1Firewall) {
            template = html`<span class="tooltip ds-u-color--success" data-tooltip="Version 1 Firewall">V1FW</span>`;
        } else if (vpcType === vpcTypeMigratingV1ToV1Firewall) {
            template = html`<span class="tooltip warn" data-tooltip="Migrating from Version 1 to Version 1 Firewall">V1&#x2192V1FW</span>`;
        } else if (vpcType === vpcTypeMigratingV1FirewallToV1) {
            template = html`<span class="tooltip warn" data-tooltip="Migrating from Version 1 Firewall to Version 1">V1FW&#x2192V1</span>`;
        }
        
        return template;
    },
    // Return a colored badge matching the colors of getStyled for textual views
     getBadge: function(vpcType) {
        let template = html`<span class="ds-c-badge">Unmanaged</span>`;
        
        if (vpcType === vpcTypeV1) {
            template = html`<span class="ds-c-badge ds-u-fill--success">Version 1</span>`;
        } else if (vpcType === vpcTypeLegacy) {
            template = html`<span class="ds-c-badge ds-u-fill--primary-darker">Legacy</span>`;
        } else if (vpcType === vpcTypeException) {
            template = html`<span class="ds-c-badge ds-c-badge--error">Exception</span>`;
        } else if (vpcType === vpcTypeV1Firewall) {
            template = html`<span class="ds-c-badge ds-u-fill--success">Version 1 Firewall</span>`;
        } else if (vpcType === vpcTypeMigratingV1ToV1Firewall) {
            template = html`<span class="ds-c-badge ds-c-badge--warn">Migrating Version 1 &#x2192 Version 1 Firewall</span>`;
        } else if (vpcType === vpcTypeMigratingV1FirewallToV1) {
            template = html`<span class="ds-c-badge ds-c-badge--warn">Migrating Version 1 Firewall &#x2192 Version 1</span>`;
        }
        
        return template;
    }
};
