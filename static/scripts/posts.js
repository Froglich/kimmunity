//A pretty decent attemp at (website) URL matching I think
let pURL = /(?<!href=\")(?<!>)(?:https?:\/\/)?((?:[a-zA-Z0-9-]+\.)+[a-z]+)(?::[0-9]+)?(?:\/[^\s,\.]+)*/g;
let pYouTube = /(?:https?:\/\/)?(?:(?:www\.)?youtube\.com\/watch\?v=|youtu\.be\/)([A-Za-z0-9-]{11})/g;

let finalPostTS = null;

function createPostSingleImageElement(image) {
    let a = createElement('a', {target: '_blank', href: `/images/${image}`, title: 'Se bilden i full storlek'});
    let img = createElement('img', {class: 'single-image', src: `/images/${image}`, alt: 'Bild tillhörande inlägget'});

    a.appendChild(img);
    return a;
}

function createPostMultiImageElement(images) {
    let pMulti = createElement('div', {class: 'multi-image'});
    let pControl = createElement('div', {class: 'image-controls'});
    let pPrev = createElement('div', {class: 'control', role: 'button', 'aria-label': 'Visa föregående bild'});
    let pNext = createElement('div', {class: 'control', role: 'button', 'aria-label': 'Visa nästa bild'});
    let pCount = createElement('div', {class: 'count'});
        pCount.innerHTML = `Bild 1 av ${images.length}`;

    pControl.appendChild(pPrev);
    pControl.appendChild(pCount);
    pControl.appendChild(pNext);
    pMulti.appendChild(pControl);

    let imageElements = [];
    for(let x = 0; x < images.length; x++) {
        let a = createElement('a', {
            target: '_blank',
            href: `/images/${images[x]}`,
            title: 'Se bilden i full storlek'
        });

        let img = createElement('img', {
            src: `/images/${images[x]}`,
            alt: `Bild ${x+1} av ${images.length}`
        });

        a.appendChild(img);
        imageElements.push(a);
    }

    pMulti.appendChild(imageElements[0]);
    let currIndex = 0;
    pPrev.setAttribute('disabled', 'disabled')
    
    let changeImage = (idx) => {
        pMulti.removeChild(imageElements[currIndex]);
        pMulti.appendChild(imageElements[idx]);
        
        currIndex = idx;
        pCount.innerHTML = `Bild ${currIndex+1} av ${images.length}`;
        if(idx == 0) {
            pPrev.setAttribute('disabled', 'disabled');
            pNext.removeAttribute('disabled');
        } else if(idx == images.length - 1) {
            pNext.setAttribute('disabled', 'disabled');
            pPrev.removeAttribute('disabled');
        } else {
            pNext.removeAttribute('disabled');
            pPrev.removeAttribute('disabled');
        }
    }

    pNext.addEventListener('click', () => {
        if(pNext.getAttribute('disabled') != 'disabled') changeImage(currIndex+1);
    });
    pPrev.addEventListener('click', () => {
        if(pPrev.getAttribute('disabled') != 'disabled') changeImage(currIndex-1);
    });

    return pMulti;
}

function formatPostImages(postData) {
    if(postData.images.length == 1) {
        return createPostSingleImageElement(postData.images[0]);
    } else {
        return createPostMultiImageElement(postData.images);
    }
}

function createPostLikeBox(postData) {
    let pLike = createElement('label', {class: 'like', 'aria-label': 'Gilla inlägget'});
    let pLCount = createElement('span', {class: 'count'});
    let pChk = createElement('input', {type: 'checkbox'});

    pLike.appendChild(pChk);
    pLike.appendChild(pLCount);

    pChk.checked = postData.liked_by_me;
    pLCount.innerHTML = postData.likes;

    pChk.addEventListener('change', () => {
        let r = new Request(`/backend/posts/${postData.post_id}/likes`);

        if(pChk.checked) {
            r.POST();
            postData.likes++;
        } else {
            r.DELETE();
            postData.likes--;
        }

        pLCount.innerHTML = postData.likes;
    });

    return pLike;
}

function createPostWriteCommentPanel(postData) {
    let pComment = createElement('div', {class: 'write-comment'});
    let pLikes = createPostLikeBox(postData);
    let pTxt = createElement('input', {type: 'text', placeholder: 'Skriv en kommentar!'});
    let pAdd = createElement('input', {type: 'button', value: 'Lägg till'});

    pAdd.addEventListener('click', () => {
        let comment = pTxt.value.trim();

        if(comment == "") {
            fancyAlert('Ser tomt ut', 'Du kan inte lägga upp en tom kommentar.');
            return
        }

        new Request(`/backend/posts/${postData.post_id}/comments`)
            .setOverlayLoaderTarget(pComment)
            .onSuccessJSON((d) => {
                let p = document.querySelector(`#post-${postData.post_id}-comments`);
                let e = createPostCommentElement(postData.post_id, d, true);
                p.appendChild(e);
                e.scrollIntoView(false);
                pTxt.value = '';                
            })
            .onError((status, msg) => {
                fancyAlert('Oväntat fel', formatStatusCode(status));
            })
            .POST({comment: comment});
    });

    pComment.appendChild(pLikes);
    pComment.appendChild(pTxt);
    pComment.appendChild(pAdd);

    return pComment;
}

function createPostCommentElement(postID, c, dyn = false) {
    let pComment = createElement('div', {class: `comment${dyn ? ' new' : ''}${c.my_comment ? ' mine' : ''}`});
    let pImg = createElement('div', {class: 'profile-pic'});
        pImg.style.backgroundImage = `url('/backend/users/${c.comment_by_username}/profile-picture')`;
    let pContent = createElement('div', {class: 'content'});
    let pUsr = createElement('a', {class: 'username', href: `/users/${c.comment_by_username}`, title: `Besök profilen tillhörande ${c.comment_by_name}`});
        pUsr.innerHTML = c.comment_by_name;
    let pTime = createElement('span', {class: 'when'});
        pTime.innerHTML = formatTimestamp(c.timestamp);
    let pText = document.createElement('span');
        pText.innerHTML = c.content;

    pContent.appendChild(pUsr);
    pContent.appendChild(document.createElement('br'));
    pContent.appendChild(pTime);
    pContent.appendChild(document.createElement('br'));
    pContent.appendChild(pText);

    pComment.appendChild(pImg);
    pComment.appendChild(pContent);

    if(c.my_comment) {
        pBtn = createElement('input', {type: 'button', class: 'context-menu'});
        pCnt = createElement('div', {class: 'context-menu-container'});
        pCtx = createElement('div', {class: 'context-menu'});
        pDel = createElement('input', {type: 'button', value: 'Ta bort kommentaren'});

        pCtx.appendChild(pDel);

        pDel.addEventListener('click', () => {
            new Request(`/backend/posts/${postID}/comments/${c.comment_id}`)
                .setLoaderParent(pComment)
                .onSuccess(() => {
                    pComment.setAttribute('class', `${pComment.getAttribute('class')} deleted`);
                    setTimeout(() => {
                        if(pComment.parentElement) pComment.parentElement.removeChild(pComment);
                    }, 800);
                })
                .onError((status, msg) => {
                    fancyAlert('Oväntat fel', `Ett oväntat fel har inträffat. ${formatStatusCode(msg)}`);
                })
                .DELETE();;
        });

        pComment.appendChild(pBtn);
        pComment.appendChild(pCtx);
    }

    return pComment;
}

function createPostCommentsPanel(postData) {
    let pComments = createElement('div', {class: 'comments'});
    let pContainer = createElement('div', {class: 'container', id: `post-${postData.post_id}-comments`});

    for(let x = 0; x < postData.comments.length; x++) {
        let c = postData.comments[x];
        pContainer.appendChild(createPostCommentElement(postData.post_id, c));
    }

    pComments.appendChild(pContainer);

    return pComments;
}

function populatePostLinkPanel(panel, domain, url) {
    let uCover = createElement('div', {class: 'cover-image'});
    let uSummary = createElement('div', {class: 'link-summary'});
    let uDomain = createElement('span', {class: 'domain important-text'})
        uDomain.innerHTML = domain;

    uSummary.appendChild(uDomain);
    panel.appendChild(uSummary);
    
    new Request('/backend/summarize-url')
        .onSuccessJSON((d) => {
            if(d.image != null) {
                uCover.style.backgroundImage = `url('${d.image}')`;
                panel.insertBefore(uCover, uSummary);
            }

            let uTitle = createElement('span', {class: 'title'});
                uTitle.innerHTML = d.title;
            uSummary.appendChild(uTitle);

            if(d.description != null) {
                let uDesc = document.createElement('span');
                    uDesc.innerHTML = d.description;
                uSummary.appendChild(uDesc);
            }
        }).GET({url: url});
}

function populatePostYouTubePanel(panel, url) {

}

function formatTimestamp(ts) {
    let currentTS = Math.round(Date.now());

    diff = currentTS - ts;
    
    if(diff < 0) {
        return 'Från framtiden';
    } else if(diff < 60000) {
        return 'För mindre än en minut sedan';
    } else if(diff < 3600000) {
        return `För ${Math.round(diff/60000)} minuter sedan`;
    } else if(diff < 86400000) {
        return `För ${Math.round(diff/3600000)} timmar sedan`;
    } else if(diff < 2629800000) {
        return `För ${Math.round(diff/86400000)} dagar sedan`;
    } else if(diff < 31557600000) {
        return `För ${Math.round(diff/2629800000)} månader sedan`;
    } else if(diff < 157788000000) {
        return `För ${Math.round(diff/31557600000)} år sedan`;
    } else {
        return 'För <b>jättelängesedan</b>';
    }
}

function buildPostElement(postData, dyn = false) {
    let pDiv = createElement('div', {class: (dyn ? 'rounded-block post new' : 'rounded-block post')});
    let pHead = createElement('div', {class: 'head'});
    let pPic = createElement('div', {class: 'profile-pic'});
        pPic.style.backgroundImage = `url('/backend/users/${postData.post_by_username}/profile-picture')`;
    let pName = createElement('a', {class: 'username', href: `/users/${postData.post_by_username}`, title: `Besök profilen för ${postData.post_by_name}`});
        pName.innerHTML = postData.post_by_name;
    let pWhen = createElement('span', {class: 'when'});
        pWhen.innerHTML = formatTimestamp(postData.posted);
    let pContent = createElement('span', {class: 'content'});

    pHead.appendChild(pPic);
    pHead.appendChild(pName);
    pHead.appendChild(pWhen);
    pDiv.appendChild(pHead);
    pDiv.appendChild(pContent);

    postData.content = postData.content.replace(/\n{2,}/, '<br><br>').replace('\n', '<br>');
    let urlMatches = [...postData.content.matchAll(pURL)];
    urlMatches.forEach((m) => {
        postData.content = postData.content.replace(m[0], `<a target="_blank" href="${m[0]}" title="${m[0]}">${m[1]}</a>`);

        let ym = [...m[0].matchAll(pYouTube)];
        if(ym.length != 0) {
            pDiv.insertAdjacentHTML('beforeend', `<iframe src="/youtube/${ym[0][1]}" frameborder="0" allow="accelerometer; autoplay; clipboard-write; encrypted-media; gyroscope; picture-in-picture" allowfullscreen></iframe>`)
        } else {
            let pLink = createElement('a', {class: 'post-link', target: '_blank', href: m[0]});
            pDiv.appendChild(pLink);
            populatePostLinkPanel(pLink, m[1], m[0]);
        }
    });
    pContent.innerHTML = postData.content;

    if(postData.my_post) {
        let pBtn = createElement('input', {type: 'button', class: 'context-menu'});
        let pCtx = createElement('div', {class: 'context-menu'});
        let pDel = createElement('input', {type: 'button', value: 'Ta bort inlägget'});

        pDel.addEventListener('click', () => {
            fancyConfirm('Helt säker?', 'Alla bilder, kommentarer och gillningar som hör till inlägget kommer att raderas, detta kan inte ångras.', () => {
                new Request(`/backend/posts/${postData.post_id}`)
                    .setLoaderParent(pDiv)
                    .onSuccess(() => {
                        pDiv.setAttribute('class', 'rounded-block post deleted');
                        setTimeout(() => {
                            if(pDiv.parentElement) pDiv.parentElement.removeChild(pDiv);
                        }, 800);
                    })
                    .onError((status, _msg) => {
                        fancyAlert('Oväntat fel', `Ett oväntat fel har inträffat. ${formatStatusCode(status)}`);
                    })
                    .DELETE();
            });
        });

        pCtx.appendChild(pDel);
        pHead.appendChild(pBtn);
        pHead.appendChild(pCtx);
    }

    if(postData.images.length > 0) {
        pDiv.appendChild(formatPostImages(postData));
    }

    pDiv.appendChild(createPostWriteCommentPanel(postData));
    pDiv.appendChild(createPostCommentsPanel(postData));

    return pDiv;
}

function getPosts(before, user, appendTo) {
    let parentElement = document.querySelector(appendTo);
    new Request('/backend/posts')
        .onSuccessJSON((d) => {
            d.forEach((e) => {
                parentElement.appendChild(buildPostElement(e));
                if(finalPostTS == null) finalPostTS = e.posted;
                else if(finalPostTS > e.posted) finalPostTS = e.posted;
            });
        })
        .onError((status, msg) => {
            console.log(status, msg);
        })
        .GET({before: before, user: user});
}

function getPost(postID, appendTo) {
    let parentElement = document.querySelector(appendTo);
    new Request(`/backend/posts/${postID}`)
        .onSuccessJSON((d) => {
            parentElement.appendChild(buildPostElement(d));
        })
        .onError((status, msg) => {
            console.log(status, msg);
        })
        .GET();
}

let wait = false;
function getMorePostsOnScroll(user, appendTo) {
    window.onscroll = () => {
        if ((window.innerHeight + window.pageYOffset) >= document.body.offsetHeight && finalPostTS != null && !wait) {
            wait = true;
            window.setTimeout(() => wait = false, 2000);
            getPosts(finalPostTS, user, appendTo);
        }
    };
}
