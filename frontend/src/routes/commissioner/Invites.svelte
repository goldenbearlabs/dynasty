<script lang="ts">
	// Franchises, their managers' accounts, and the invite links that create them.
	import { invalidateAll } from '$app/navigation';
	import { addFranchise, getInvites, reissueInvite, updateFranchise, type Invite } from '#lib/api.ts';
	import Crest from '#lib/ui/Crest.svelte';
	import Icon from '#lib/ui/Icon.svelte';
	import { toast } from '#lib/ui/toast.svelte.ts';

	let invites = $state<Invite[]>([]);
	let fresh = $state({ name: '', manager_name: '' });

	const link = (invite: Invite) => `${location.origin}/join/${invite.invite_token}`;

	// Runs a change, then refreshes this list and the rest of the app.
	async function run(work: () => Promise<unknown>, done?: string) {
		try {
			await work();
			invites = await getInvites();
			await invalidateAll();
			if (done) toast.good(done);
		} catch (e) {
			toast.error(e);
		}
	}
	run(async () => {});

	async function copy(invite: Invite) {
		await navigator.clipboard.writeText(link(invite));
		toast.good(`Copied ${invite.manager_name}'s invite link.`);
	}

	// A new link for a manager who has an account lets them choose new
	// credentials, which is how a forgotten password is fixed.
	function newLink(invite: Invite) {
		const reset = invite.email !== '';
		if (reset && !confirm(`Create a link that lets ${invite.manager_name} choose a new email and password?`)) return;
		run(() => reissueInvite(invite.id), reset ? 'Reset link created. Copy it and send it to them.' : 'New invite link created.');
	}

	function add(event: SubmitEvent) {
		event.preventDefault();
		run(async () => {
			await addFranchise(fresh.name, fresh.manager_name);
			fresh = { name: '', manager_name: '' };
		}, 'Franchise added.');
	}
</script>

<section class="stack tight">
	<p class="muted">
		Send each manager their invite link. Opening it lets them create their account; the link then stops working. If someone
		forgets their password, create a new link for them.
	</p>

	<div class="card flush scroll">
		<table>
			<thead>
				<tr><th>Franchise</th><th>Manager</th><th>Commissioner</th><th>Account</th><th></th></tr>
			</thead>
			<tbody>
				{#each invites as invite (invite.id)}
					<tr>
						<td>
							<div class="row name">
								<Crest src={invite.image_url} name={invite.name} size={30} />
								<input aria-label="Franchise name" bind:value={invite.name} />
							</div>
						</td>
						<td><input aria-label="Manager" bind:value={invite.manager_name} /></td>
						<td><input type="checkbox" aria-label="Commissioner" bind:checked={invite.is_commissioner} /></td>
						<td class="links">
							{#if invite.email}<span class="small-text">{invite.email}</span>{:else}<span class="pill gold">No account yet</span>{/if}
							{#if invite.invite_token}
								<button class="small" onclick={() => copy(invite)}><Icon name="link" size={14} /> Copy invite link</button>
							{:else}
								<button class="small quiet" onclick={() => newLink(invite)}>Reset access</button>
							{/if}
						</td>
						<td class="actions"><button class="small" onclick={() => run(() => updateFranchise(invite), 'Saved.')}>Save</button></td>
					</tr>
				{/each}
			</tbody>
		</table>
	</div>

	<form class="card row end" onsubmit={add}>
		<label class="field grow">New franchise name <input bind:value={fresh.name} required /></label>
		<label class="field grow">Manager <input bind:value={fresh.manager_name} required /></label>
		<button><Icon name="plus" size={16} /> Add franchise</button>
	</form>
</section>

<style>
	.name {
		flex-wrap: nowrap;
	}
	td input:not([type='checkbox']) {
		width: 100%;
		min-width: 8rem;
	}
	.links {
		white-space: nowrap;
	}
	.links > * + * {
		margin-left: 0.5rem;
	}
	.grow {
		flex: 1 1 12rem;
	}
</style>
